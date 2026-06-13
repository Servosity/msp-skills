# Liongard CLI

**Every Liongard endpoint, plus an offline copy of your whole MSP estate you can join, search, and drift-check from one command.**

Liongard's API answers one environment at a time and only through the web UI or per-instance REST calls. This CLI syncs every environment, system, launchpoint, inspector, agent, detection, metric, and timeline entry into a local SQLite store, then runs cross-estate joins the live API never returns: drift since a window, stale launchpoints, offline agents, monitoring-coverage gaps, and metric pivots. Agent-native throughout  -  --json, --select, --csv, and typed exit codes.

## Install

The recommended path installs both the `liongard-cli` binary and the `pp-liongard` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install liongard
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install liongard --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install liongard --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install liongard --agent claude-code
npx -y @mvanhorn/printing-press-library install liongard --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/liongard/cmd/liongard-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/liongard-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install liongard --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-liongard --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-liongard --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install liongard --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/liongard-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `LIONGARD_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/liongard/cmd/liongard-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "liongard": {
      "command": "liongard-mcp",
      "env": {
        "LIONGARD_INSTANCE": "<instance>",
        "LIONGARD_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Liongard issues an Access Key ID and an Access Key Secret per user. The CLI sends them as the X-ROAR-API-KEY header (base64 of `accessKeyId:accessKeySecret`). Set LIONGARD_INSTANCE (your subdomain, e.g. us1), LIONGARD_ACCESS_KEY_ID, and LIONGARD_ACCESS_KEY_SECRET; or set a pre-encoded LIONGARD_API_KEY directly. Run `doctor` to confirm the host and credentials resolve.

## Quick Start

```bash
# Confirm instance + credentials resolve before anything else.
liongard-cli doctor

# Pull the whole estate into the local store so offline joins work.
liongard-cli sync --full

# List every client environment as structured JSON.
liongard-cli environments get --json

# See what changed across all clients in the last day.
liongard-cli drift --since 24h

# Find collectors that stopped reporting.
liongard-cli launchpoints stale --older-than 7d

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Whole-estate visibility
- **`drift`**  -  Every change detected across all your client environments within a time window, joined to the owning environment and system.

  _Reach for this when an agent needs the whole-estate 'what changed overnight' answer instead of paging the per-environment detections endpoint._

  ```bash
  liongard-cli drift --since 24h --agent
  ```
- **`environments overview`**  -  One client's complete picture in a single command: its systems, each system's launchpoints, agent, latest inspection, open detections, and key metrics.

  _Use when an agent is asked about a specific client and needs the joined picture rather than five separate endpoint calls._

  ```bash
  liongard-cli environments overview 42 --full --agent
  ```
- **`systems history`**  -  The full chronological change history of one system: every detection and inspection entry in time order.

  _Reach for this when an agent needs 'when did this system change / break' instead of paging timeline and detections endpoints separately._

  ```bash
  liongard-cli systems history 4821 --agent
  ```

### Inspection health
- **`launchpoints stale`**  -  Every launchpoint whose newest inspection is older than a threshold, with the owning environment and system named.

  _Use to find collectors that silently stopped reporting before a client notices their data is old._

  ```bash
  liongard-cli launchpoints stale --older-than 7d --agent
  ```
- **`agents offline`**  -  Every offline agent across the estate, joined to the environment it serves.

  _Use for the daily 'is everything still collecting' health sweep across all clients at once._

  ```bash
  liongard-cli agents offline --agent
  ```
- **`coverage`**  -  Monitoring gaps: systems with no launchpoint bound, and environments with no systems at all.

  _Use during onboarding QA to catch clients you provisioned but never pointed an inspector at._

  ```bash
  liongard-cli coverage --agent
  ```
- **`launchpoints run-stale`**  -  Find stale launchpoints and trigger an inspection run on each, in one guarded command.

  _Use to re-kick every collector that fell behind, instead of clicking run on each launchpoint in the UI._

  ```bash
  liongard-cli launchpoints run-stale --older-than 7d
  ```
- **`detections failures`**  -  Every inspection that ran but failed or errored across the estate, joined to the owning environment.

  _Reach for this to find inspections that ran but errored - a different failure mode from collectors that silently stopped reporting (launchpoints stale)._

  ```bash
  liongard-cli detections failures --since 7d --agent
  ```
- **`inspectors coverage`**  -  Which environments are missing a given inspector type - the estate-wide rollout-gap view.

  _Reach for this when an agent needs 'who is missing the M365 inspector' instead of paging launchpoints per environment._

  ```bash
  liongard-cli inspectors coverage --inspector "Microsoft 365" --agent
  ```
- **`health`**  -  One estate-wide health scorecard: stale launchpoints, offline agents, failed inspections, and coverage gaps as a single summary with a typed exit code.

  _Reach for this for a single-command daily health check with a typed exit code, instead of running each sweep separately._

  ```bash
  liongard-cli health --agent
  ```

### Reporting and SLA
- **`metrics pivot`**  -  One RoarPath metric pulled across every system as a system-by-value table, CSV-ready for reports.

  _Use to assemble a single metric across the whole estate for an SLA or QBR deck in one command._

  ```bash
  liongard-cli metrics pivot "MFA Enabled Count" --csv
  ```
- **`metrics breach`**  -  Every system whose RoarPath metric value crosses a numeric threshold.

  _Use for SLA-breach and security-posture checks across all clients without N per-system calls._

  ```bash
  liongard-cli metrics breach "Patch Age Days" --op gt --value 30 --agent
  ```

## Recipes


### Morning drift triage as JSON

```bash
liongard-cli drift --since 24h --agent --select environment,system,name
```

Whole-estate change feed for the last day, projected to just the fields an agent needs.

### Estate-wide stale collector report

```bash
liongard-cli launchpoints stale --older-than 7d --csv
```

CSV of every launchpoint that stopped reporting, ready to paste into a ticket.

### Pivot one metric across all systems

```bash
liongard-cli metrics pivot "MFA Enabled Count" --csv
```

A system-by-value table for a single RoarPath metric across the estate.

### Narrow a verbose detections payload

```bash
liongard-cli detections detections --agent --select ID,EnvironmentID,Name
```

Detections responses are large; --select keeps only the high-gravity fields so an agent does not burn context.

### Find security-posture breaches

```bash
liongard-cli metrics breach "Patch Age Days" --op gt --value 30 --agent
```

Every system whose patch age crosses the threshold, across all clients.

## Usage

Run `liongard-cli --help` for the full command reference and flag list.

## Commands

### access-keys

Manage access keys

- **`liongard-cli access-keys create`** - Create an Access Token with the permission of user or with only 'Add Agent' permission
- **`liongard-cli access-keys delete`** - Delete Access Token created by user
- **`liongard-cli access-keys get`** - Returns a List of Access Tokens created by user
- **`liongard-cli access-keys get-count`** - Return a count of all Access Tokens created by user

### agents

Agents can be installed in the Cloud or On-Premise and are responsible for running inspections.

- **`liongard-cli agents delete`** - Remove an agent
- **`liongard-cli agents get`** - List all agents.
- **`liongard-cli agents get-agentid`** - Get a specific Agent.
- **`liongard-cli agents get-count`** - Returns a count of all the agents in your service provider.
- **`liongard-cli agents update`** - Edits a deployed liongard agent, cannot update On-Demand agents.

### authentication

Manage authentication

- **`liongard-cli authentication log-in`** - Returns a session token as well as additional information about logged-in user
- **`liongard-cli authentication verify-token`** - Verify token with MFA authentication code

### detections

A Detection is a Change that was detected on a system.

- **`liongard-cli detections detections`** - Returns a list of all detection events.
- **`liongard-cli detections detectionsby-id`** - Gets a specific detection.
- **`liongard-cli detections get-count1234`** - Returns count of all detection events.

### environments

An environment in Roar represents a single end customer for your MSP. An environment will contain Agents, Launchpoints (configured inspections), and Systems (data that has landed as a result of an inspection).

- **`liongard-cli environments count`** - Returns a count of all environments in your Service Provider.
- **`liongard-cli environments create`** - Creates a single Liongard Environment. If you want to create a child environment, set the Parent Environment, if not set to null.
- **`liongard-cli environments create-bulk`** - Create many environments at once. If you want to create a child environment, set the Parent Environment, if not set to null
- **`liongard-cli environments delete`** - Delete an environment.
- **`liongard-cli environments get`** - Fetch all environments in your Service Provider.
- **`liongard-cli environments get-environmentid`** - Get a single environment.
- **`liongard-cli environments update`** - Update a single environment, If you want to update a child environment, set the Parent Environment, if not set to null

### groups

A Group represents a combined set of permissions that can be assigned to Users

- **`liongard-cli groups`** - Returns a List of available Assignable Roles for a user

### inspector

An Inspector represents the system type and used for building the config templates for launchpoints


### inspectors

An Inspector represents the system type and used for building the config templates for launchpoints

- **`liongard-cli inspectors`** - Lists all avaialble Inspectors in Liongard

### launchpoints

A Launchpoint is a configured Inspection. As an example, if you set up a domain inspector to run on liongard.com, that would be considered a Launchpoint.

- **`liongard-cli launchpoints bulk-delete`** - Remove all launchpoints.
- **`liongard-cli launchpoints bulk-run`** - Kick off many inspections.
- **`liongard-cli launchpoints delete`** - Remove a single launchpoint.
- **`liongard-cli launchpoints get`** - Lists all launchpoints.
- **`liongard-cli launchpoints get-count`** - Returns a count of all launchpoints.
- **`liongard-cli launchpoints launchpoint`** - Create a launchpoint. You will need to reference the inspector templates for building out the config and secure config objects for this launchpoint. Each Inspector has a different template that contains the necessary configuration fields.
- **`liongard-cli launchpoints launchpointsby-id`** - Return a specific launchpoint by ID.
- **`liongard-cli launchpoints update`** - Edit a single inspector launchpoint. You will need to reference the inspector templates for building out the config and secure config objects for this launchpoint. Each Inspector has a different template that contains the necessary configuration fields.
- **`liongard-cli launchpoints update-bulk`** - Update many launchpoints to run on the same schedule.

### logs

Manage logs

- **`liongard-cli logs`** - Return the logs for a specific inspection.

### metrics

Manage metrics

- **`liongard-cli metrics create`** - Creates a Single Metrics for a system requires valid JMESPath in the Query field
- **`liongard-cli metrics delete`** - Deletes only Custom Created Metrics, Liongard Created Metrics can not be deleted
- **`liongard-cli metrics evaluation`** - For each system ID passed, evaluates each metric ID/UUID passed for that system for the latest inspection data and returns the results. You can use this to build your own reports. You must pass a list of system IDs and metric IDs/UUIDs in the URL parameters. You can request values for up to 10 systems at a time.  Setting includeNonVisible true allows you to get metric values without requiring you to toggle metrics to display within the Liongard UI.
- **`liongard-cli metrics evaluation-post`** - For each system ID passed, evaluates each metric ID/UUID passed for that system for the latest inspection data and returns the results. Instead of the latest inspection data, you can specify a date from which to retrieve metric values. Setting includeNonVisible true allows you to get metric values without requiring you to toggle metrics to display within the Liongard UI.
- **`liongard-cli metrics metrics`** - Returns a list of metrics that have been created.
- **`liongard-cli metrics update`** - Updates a Metric requires valid JMESPath in the Query field

### systems

A System in Roar represents a system that has been inspected by a Launchpoint. When the inspection completes for the first time, a system is completed and a corresponding timeline entry is created each time the inspection lands.

- **`liongard-cli systems get-count`** - Count of all systems in your service provider.
- **`liongard-cli systems systems`** - List all systems.

### tasks

Manage tasks

- **`liongard-cli tasks get-alert`** - Returns a single alert that has been raised.
- **`liongard-cli tasks get-count12345`** - Returns a count of all alerts.
- **`liongard-cli tasks list-alerts`** - Returns a list of alerts that have been raised.

### timeline

A Timeline entry represents a single inspection event. It links a System to it's configuration at that point in time.

- **`liongard-cli timeline get-count123`** - Return count of all timeline entries.
- **`liongard-cli timeline timeline`** - Fetch all timeline entries.
- **`liongard-cli timeline timelineby-id`** - Fetch a specific timeline.

### users

Create and manage users in your Liongard instance

- **`liongard-cli users count`** - Returns a count of total users
- **`liongard-cli users create`** - When creating and using the Manger or Reader Roles it is required to use either the Environment/EnvironmentID or EnvironmentGroupID/EnvironmentGroup to select environments associated to each role. If selecting Admin, System Integrator or User admin keep these fields null.
- **`liongard-cli users delete`** - Remove a single User.
- **`liongard-cli users get`** - Returns a list of users in your Liongard Instance
- **`liongard-cli users get-single`** - Returns a Single User
- **`liongard-cli users update`** - Updates a single User


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
liongard-cli access-keys get

# JSON for scripting and agents
liongard-cli access-keys get --json

# Filter to specific fields
liongard-cli access-keys get --json --select id,name,status

# Dry run  -  show the request without sending
liongard-cli access-keys get --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
liongard-cli access-keys get --agent
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

## Runtime Endpoint

This CLI resolves endpoint placeholders at runtime, so one installed binary can target different tenants or API versions without regeneration.

Endpoint environment variables:
- `LIONGARD_INSTANCE` resolves `{instance}`

Base URL: `https://{instance}.app.liongard.com/api/v1`

## Health Check

```bash
liongard-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/liongard-endpoints-pp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `LIONGARD_INSTANCE` | endpoint | Yes |  |
| `LIONGARD_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `liongard-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `liongard-cli doctor` to check credentials
- Verify the environment variable is set: `echo $LIONGARD_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **doctor reports the host cannot be resolved**  -  Set LIONGARD_INSTANCE to your subdomain only (e.g. us1), not the full URL; the base becomes https://us1.app.liongard.com/api/v1.
- **401 Unauthorized on every call**  -  The X-ROAR-API-KEY must be base64 of accessKeyId:accessKeySecret. Set LIONGARD_ACCESS_KEY_ID and LIONGARD_ACCESS_KEY_SECRET and let the CLI encode them, or set a correctly pre-encoded LIONGARD_API_KEY.
- **drift or stale commands return nothing**  -  Run `sync --full` first; the cross-estate joins read the local store, not the live API.
- **list commands return a Data wrapper you do not want**  -  The CLI already unwraps the {Data,Pagination,Success} envelope; use --select to project just the fields you need.
