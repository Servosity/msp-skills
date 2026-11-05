# Liongard CLI

**Every Liongard endpoint, plus an offline copy of your whole MSP estate you can join, search, and drift-check from one command.**

Liongard's API answers one environment at a time and only through the web UI or per-instance REST calls. This CLI syncs every environment, system, launchpoint, inspector, agent, detection, metric, and timeline entry into a local SQLite store, then runs cross-estate joins the live API never returns: drift since a window, stale launchpoints, offline agents, monitoring-coverage gaps, and metric pivots. Agent-native throughout  -  --json, --select, --csv, and typed exit codes.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `liongard-cli` and `liongard-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/liongard/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/liongard/install.ps1 | iex
   ```
3. Verify: `liongard-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=liongard). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/liongard --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/liongard --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `liongard-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the liongard skill from https://github.com/Servosity/msp-skills/tree/main/skills/liongard. The skill defines how its required CLI (`liongard-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=liongard).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `LIONGARD_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/liongard/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/liongard/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

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

### Remote MCP over HTTP

`liongard-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `LIONGARD_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `LIONGARD_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export LIONGARD_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
liongard-mcp --transport http
```

Set `LIONGARD_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

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
- **`metrics pivot`**  -  One RoarPath metric across every system it covers, CSV-ready for reports; the value column is filled only where the local store carries an evaluated value.

  _Use to assemble a single metric across the whole estate for an SLA or QBR deck in one command._

  ```bash
  liongard-cli metrics pivot "MFA Enabled Count" --csv
  ```
- **`metrics breach`**  -  Every system whose RoarPath metric value crosses a numeric threshold. It refuses until every covered system has an evaluated value in the local store; the synced metric definitions carry none (Liongard evaluates them per system: `liongard-cli metrics evaluation`).

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

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `LIONGARD_CONFIG_DIR`, `LIONGARD_DATA_DIR`, `LIONGARD_STATE_DIR`, or `LIONGARD_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `LIONGARD_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export LIONGARD_HOME=/srv/liongard
liongard-cli doctor
```

On Windows (PowerShell):

```powershell
$env:LIONGARD_HOME = "C:\ProgramData\liongard"
liongard-cli doctor
```

Under `LIONGARD_HOME=/srv/liongard`, the four dirs resolve to `/srv/liongard/config`, `/srv/liongard/data`, `/srv/liongard/state`, and `/srv/liongard/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:LIONGARD_HOME = "C:\srv\liongard"
liongard-cli doctor
```

Under `$env:LIONGARD_HOME = "C:\srv\liongard"`, the four dirs resolve to `C:\srv\liongard\config`, `C:\srv\liongard\data`, `C:\srv\liongard\state`, and `C:\srv\liongard\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "liongard": {
      "command": "liongard-mcp",
      "env": {
        "LIONGARD_HOME": "/srv/liongard"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `LIONGARD_DATA_DIR` overrides an explicit `--home` for that kind. Use `LIONGARD_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `LIONGARD_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `liongard-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are never cached on disk. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `liongard-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `liongard-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `LIONGARD_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `LIONGARD_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

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


### agents


- **`liongard-cli agents offline`** - Every offline agent across the estate, joined to the environment it serves.


### coverage


- **`liongard-cli coverage`** - Monitoring gaps: systems with no launchpoint bound, and environments with no systems at all.


### detections


- **`liongard-cli detections failures`** - Every inspection that ran but failed or errored across the estate, joined to the owning environment.


### drift


- **`liongard-cli drift`** - Every change detected across all your client environments within a time window, joined to the owning environment and system.


### environments


- **`liongard-cli environments overview`** - One client's complete picture in a single command: its systems, each system's launchpoints, agent, latest inspection, open detections, and key metrics.


### health


- **`liongard-cli health`** - One estate-wide health scorecard: stale launchpoints, offline agents, failed inspections, and coverage gaps as a single summary with a typed exit code.


### inspectors


- **`liongard-cli inspectors coverage`** - Which environments are missing a given inspector type - the estate-wide rollout-gap view.


### launchpoints


- **`liongard-cli launchpoints stale`** - Every launchpoint whose newest inspection is older than a threshold, with the owning environment and system named.

- **`liongard-cli launchpoints run-stale`** - Find stale launchpoints and trigger an inspection run on each, in one guarded command.


### metrics


- **`liongard-cli metrics pivot`** - One RoarPath metric across every system it covers, CSV-ready for reports; the value column is filled only where the local store carries an evaluated value.

- **`liongard-cli metrics breach`** - Every system whose RoarPath metric value crosses a numeric threshold. It refuses until every covered system has an evaluated value in the local store; the synced metric definitions carry none (Liongard evaluates them per system: `liongard-cli metrics evaluation`).


### systems


- **`liongard-cli systems history`** - The full chronological change history of one system: every detection and inspection entry in time order.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`liongard-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`liongard-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`liongard-cli learnings list`** - Inspect taught rows
- **`liongard-cli learnings forget <query>`** - Undo a teach
- **`liongard-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`liongard-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`liongard-cli teach-pattern`** - Install a query/resource template up front
- **`liongard-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `LIONGARD_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `liongard-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
liongard-cli access-keys get

# JSON for scripting and agents
liongard-cli access-keys get --json
# Filter to specific fields
liongard-cli access-keys get --json --select AccessKeyId,CreatedOn,DeletedOn

# Dry run  -  show the request without sending
liongard-cli access-keys get --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
liongard-cli access-keys get --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **Explicit confirmation** - `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

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

Run `liongard-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/liongard-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\liongard-cli\config.toml` on Windows; `--home`, `LIONGARD_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `LIONGARD_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `LIONGARD_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `LIONGARD_INSTANCE` | endpoint | Yes |  |
| `LIONGARD_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `liongard-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Credential safety

- The `X-ROAR-API-KEY` header (and the `X-Auth-Token` session token) and every API request go only to the scheme, host and port of the resolved base URL (`https://<LIONGARD_INSTANCE>.app.liongard.com/api/v1`, or `LIONGARD_BASE_URL`). A redirect to any other host is refused, so the credential never reaches it.
- Any output, file or MCP result that would repeat `LIONGARD_API_KEY`, the Access Key ID or Secret, the header composed from them, the session token, or a credential-named value from a response (a new access key's secret, for example) prints "[text withheld: it repeated a secret value this program holds]" in its place. Responses are never cached on disk, and credential-named fields are dropped before anything reaches the local store.
- Only `credentials.toml` (mode 0600) holds a credential; `auth set-token` reads the token from stdin, never from an argument.
- `--dry-run` sends nothing. Over MCP, a tool that creates, changes, deletes or runs something in Liongard (`liongard_execute`, `launchpoints run-stale`, `import`) returns a preview and sends only with `confirm: true`, and a tool argument can never name a file to read or write.

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
- **drift, stale or offline commands refuse with "sync never finished" or a narrowed copy**  -  Run the `sync --full --resources <name>` command the message names, without `--param` or `--resource-param`; the cross-estate joins read the local store, not the live API.
- **list commands return a Data wrapper you do not want**  -  The CLI already unwraps the {Data,Pagination,Success} envelope; use --select to project just the fields you need.
