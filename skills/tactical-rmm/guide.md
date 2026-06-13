# Tactical RMM CLI

**Every Tactical RMM endpoint as a typed command, plus an offline SQLite mirror and cross-entity fleet queries the web UI can't express.**

A terminal-native, agent-native control plane for self-hosted Tactical RMM. Mirror your fleet into local SQLite, run cross-entity queries (fleet health, triage, patch posture, coverage) that no single API call returns, fan scripts out across a filtered cohort, and pipe clean JSON anywhere.

## Install

The recommended path installs both the `tactical-rmm-cli` binary and the `pp-tactical-rmm` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install tactical-rmm
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install tactical-rmm --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install tactical-rmm --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install tactical-rmm --agent claude-code
npx -y @mvanhorn/printing-press-library install tactical-rmm --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/tactical-rmm/cmd/tactical-rmm-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/tactical-rmm-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install tactical-rmm --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-tactical-rmm --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-tactical-rmm --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install tactical-rmm --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/tactical-rmm-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `TRMM_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/tactical-rmm/cmd/tactical-rmm-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "tactical-rmm": {
      "command": "tactical-rmm-mcp",
      "env": {
        "TRMM_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Authenticate with a Tactical RMM API key in the X-API-KEY header (Settings > Global Settings > API Keys; bypasses 2FA). Set TRMM_API_KEY and point the CLI at your self-hosted API host (e.g. https://api.your-domain.com). No tenant is baked in.

## Quick Start

```bash
# Confirm the API key and host are reachable.
tactical-rmm-cli doctor

# Mirror agents, clients, checks, alerts, inventory into the local store.
tactical-rmm-cli sync

# One-shot posture of the whole fleet.
tactical-rmm-cli fleet health

# See which agents need attention first.
tactical-rmm-cli triage --limit 20

# Offline FTS across synced entities.
tactical-rmm-cli search <query>

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Cross-entity fleet intelligence
- **`fleet health`**  -  Whole-fleet posture in one command: online/offline/overdue agents, failing checks, pending reboots, outstanding patches, active alerts.

  _Size up an environment before drilling in._

  ```bash
  tactical-rmm-cli fleet health
  ```
- **`triage`**  -  Ranked list of agents needing attention, scored across offline state, failing checks, reboots and patches.

  _Decide what to fix first across hundreds of endpoints._

  ```bash
  tactical-rmm-cli triage --limit 20
  ```
- **`agents stale`**  -  Agents whose last check-in exceeds a threshold, with client and site.

  _Find abandoned endpoints before they become a gap._

  ```bash
  tactical-rmm-cli agents stale --days 7
  ```
- **`since`**  -  What changed across the fleet since a window: new alerts, newly-offline agents.

  _Start a shift seeing only what moved._

  ```bash
  tactical-rmm-cli since 2h
  ```
- **`clients scorecard`**  -  One posture row per client: agent count, online share, failing checks, pending patches, open alerts.

  _Use for per-customer health rollups and QBR prep instead of clicking through every client._

  ```bash
  tactical-rmm-cli clients scorecard --agent
  ```

### Patch & inventory
- **`patch posture`**  -  Per-client/site rollup of pending Windows updates and reboots.

  _Answer who is behind on patches in one call._

  ```bash
  tactical-rmm-cli patch posture --by client
  ```
- **`software find`**  -  Which agents have a given software package installed across the fleet.

  _Answer who is exposed during a CVE scramble._

  ```bash
  tactical-rmm-cli software find <name>
  ```
- **`services down`**  -  Agents where a named Windows service is stopped, across the whole fleet. Live fan-out: requires TRMM_API_KEY (agent list comes from the local store).

  _Use during incidents to find every endpoint with a critical service stopped._

  ```bash
  tactical-rmm-cli services down --name Spooler --agent
  ```

### Remote execution
- **`agents bulk-run`**  -  Run a script/command across every agent matching a local-store filter (client/site/os/online).

  _Push a fix or audit to a whole cohort at once._

  ```bash
  tactical-rmm-cli agents bulk-run --command whoami --filter os=windows,online=true
  ```
- **`maintenance set`**  -  Put a filtered cohort of agents into maintenance mode in one command, with a reminder window (re-run with --clear to end it; Tactical RMM has no server-side expiry).

  _Reach for this when silencing monitoring for a planned window across many agents at once; previews by default, mutates only with --execute and a live TRMM_API_KEY._

  ```bash
  tactical-rmm-cli maintenance set --filter site=HQ --until 4h
  ```
- **`actions pending`**  -  Queued agent actions across the fleet grouped by agent and age, so stuck dispatches surface. Live fan-out: requires TRMM_API_KEY (agent list comes from the local store).

  _Run after a bulk dispatch to see which agents haven't returned results._

  ```bash
  tactical-rmm-cli actions pending --agent
  ```

### Monitoring quality
- **`checks flapping`**  -  Checks that repeatedly flipped pass/fail within a window, from local snapshots.

  _Catch noisy monitors that erode alert trust._

  ```bash
  tactical-rmm-cli checks flapping --window 24h
  ```
- **`alerts digest`**  -  Grouped alert summary by client/severity/type over a time window.

  _Turn an alert firehose into a shift-handoff summary._

  ```bash
  tactical-rmm-cli alerts digest --since 24h
  ```
- **`coverage`**  -  Agents missing checks (unmonitored endpoints).

  _Find unmonitored endpoints before they fail silently._

  ```bash
  tactical-rmm-cli coverage
  ```
- **`checks worst`**  -  Failing checks ranked by blast radius - how many agents are red on each check.

  _Use this to find the single failure hurting the most endpoints before triaging agent-by-agent._

  ```bash
  tactical-rmm-cli checks worst --limit 10 --agent
  ```

## Recipes


### Morning fleet triage

```bash
tactical-rmm-cli triage --limit 25
```

Ranked attention list across offline state, failing checks, reboots, patches.

### CVE exposure sweep

```bash
tactical-rmm-cli software find <name> --select agent_id,name,version
```

Which agents carry a package, narrowed to the fields that matter.

### Patch posture by client

```bash
tactical-rmm-cli patch posture --by client
```

Roll pending Windows updates up to each customer.

### Shift handoff: what changed

```bash
tactical-rmm-cli since 8h
```

New alerts and newly-offline agents since your last shift.

## Usage

Run `tactical-rmm-cli --help` for the full command reference and flag list.

## Commands

### accounts

Manage accounts

- **`tactical-rmm-cli accounts accounts`** - GET /accounts/apikeys/
- **`tactical-rmm-cli accounts accounts-apikeys`** - POST /accounts/apikeys/
- **`tactical-rmm-cli accounts accounts-apikeys-2`** - DELETE /accounts/apikeys/{pk}/
- **`tactical-rmm-cli accounts accounts-apikeys-3`** - PUT /accounts/apikeys/{pk}/
- **`tactical-rmm-cli accounts accounts-reset2fa`** - PUT /accounts/reset2fa/
- **`tactical-rmm-cli accounts accounts-resetpw`** - PUT /accounts/resetpw/
- **`tactical-rmm-cli accounts accounts-roles`** - GET /accounts/roles/
- **`tactical-rmm-cli accounts accounts-roles-2`** - POST /accounts/roles/
- **`tactical-rmm-cli accounts accounts-roles-3`** - DELETE /accounts/roles/{pk}/
- **`tactical-rmm-cli accounts accounts-roles-4`** - GET /accounts/roles/{pk}/
- **`tactical-rmm-cli accounts accounts-roles-5`** - PUT /accounts/roles/{pk}/
- **`tactical-rmm-cli accounts accounts-sessions`** - DELETE /accounts/sessions/{pk}/
- **`tactical-rmm-cli accounts accounts-users`** - GET /accounts/users/
- **`tactical-rmm-cli accounts accounts-users-10`** - GET /accounts/users/{pk}/sessions/
- **`tactical-rmm-cli accounts accounts-users-2`** - POST /accounts/users/
- **`tactical-rmm-cli accounts accounts-users-3`** - POST /accounts/users/reset/
- **`tactical-rmm-cli accounts accounts-users-4`** - PUT /accounts/users/reset/
- **`tactical-rmm-cli accounts accounts-users-5`** - POST /accounts/users/reset_totp/
- **`tactical-rmm-cli accounts accounts-users-6`** - PUT /accounts/users/reset_totp/
- **`tactical-rmm-cli accounts accounts-users-7`** - POST /accounts/users/setup_totp/
- **`tactical-rmm-cli accounts accounts-users-8`** - PATCH /accounts/users/ui/
- **`tactical-rmm-cli accounts accounts-users-9`** - DELETE /accounts/users/{pk}/sessions/

### agents

Manage agents

- **`tactical-rmm-cli agents agents`** - GET /agents/
- **`tactical-rmm-cli agents agents-actions`** - GET /agents/actions/bulk/
- **`tactical-rmm-cli agents agents-agentid`** - DELETE /agents/{agent_id}/
- **`tactical-rmm-cli agents agents-agentid-2`** - GET /agents/{agent_id}/
- **`tactical-rmm-cli agents agents-agentid-3`** - PUT /agents/{agent_id}/
- **`tactical-rmm-cli agents agents-bulkrecovery`** - GET /agents/bulkrecovery/
- **`tactical-rmm-cli agents agents-history`** - GET /agents/history/
- **`tactical-rmm-cli agents agents-installer`** - GET /agents/installer/
- **`tactical-rmm-cli agents agents-maintenance`** - GET /agents/maintenance/bulk/
- **`tactical-rmm-cli agents agents-notes`** - GET /agents/notes/
- **`tactical-rmm-cli agents agents-notes-2`** - POST /agents/notes/
- **`tactical-rmm-cli agents agents-notes-3`** - DELETE /agents/notes/{pk}/
- **`tactical-rmm-cli agents agents-notes-4`** - GET /agents/notes/{pk}/
- **`tactical-rmm-cli agents agents-notes-5`** - PUT /agents/notes/{pk}/
- **`tactical-rmm-cli agents agents-scripthistory`** - GET /agents/scripthistory/
- **`tactical-rmm-cli agents agents-update`** - GET /agents/update/
- **`tactical-rmm-cli agents agents-versions`** - GET /agents/versions/

### alerts

Manage alerts

- **`tactical-rmm-cli alerts alerts`** - PATCH /alerts/
- **`tactical-rmm-cli alerts alerts-bulk`** - POST /alerts/bulk/
- **`tactical-rmm-cli alerts alerts-endpoint`** - POST /alerts/
- **`tactical-rmm-cli alerts alerts-pk`** - DELETE /alerts/{pk}/
- **`tactical-rmm-cli alerts alerts-pk-2`** - GET /alerts/{pk}/
- **`tactical-rmm-cli alerts alerts-pk-3`** - PUT /alerts/{pk}/
- **`tactical-rmm-cli alerts alerts-templates`** - GET /alerts/templates/
- **`tactical-rmm-cli alerts alerts-templates-2`** - POST /alerts/templates/
- **`tactical-rmm-cli alerts alerts-templates-3`** - DELETE /alerts/templates/{pk}/
- **`tactical-rmm-cli alerts alerts-templates-4`** - GET /alerts/templates/{pk}/
- **`tactical-rmm-cli alerts alerts-templates-5`** - PUT /alerts/templates/{pk}/
- **`tactical-rmm-cli alerts alerts-templates-6`** - GET /alerts/templates/{pk}/related/

### automation

Manage automation

- **`tactical-rmm-cli automation automation`** - DELETE /automation/patchpolicy/
- **`tactical-rmm-cli automation automation-checks`** - GET /automation/checks/{check}/status/
- **`tactical-rmm-cli automation automation-patchpolicy`** - POST /automation/patchpolicy/
- **`tactical-rmm-cli automation automation-patchpolicy-2`** - PUT /automation/patchpolicy/
- **`tactical-rmm-cli automation automation-patchpolicy-3`** - POST /automation/patchpolicy/reset/
- **`tactical-rmm-cli automation automation-patchpolicy-4`** - DELETE /automation/patchpolicy/{pk}/
- **`tactical-rmm-cli automation automation-patchpolicy-5`** - POST /automation/patchpolicy/{pk}/
- **`tactical-rmm-cli automation automation-patchpolicy-6`** - PUT /automation/patchpolicy/{pk}/
- **`tactical-rmm-cli automation automation-policies`** - GET /automation/policies/
- **`tactical-rmm-cli automation automation-policies-2`** - POST /automation/policies/
- **`tactical-rmm-cli automation automation-policies-3`** - GET /automation/policies/overview/
- **`tactical-rmm-cli automation automation-policies-4`** - DELETE /automation/policies/{pk}/
- **`tactical-rmm-cli automation automation-policies-5`** - GET /automation/policies/{pk}/
- **`tactical-rmm-cli automation automation-policies-6`** - PUT /automation/policies/{pk}/
- **`tactical-rmm-cli automation automation-policies-7`** - GET /automation/policies/{pk}/related/
- **`tactical-rmm-cli automation automation-policies-8`** - GET /automation/policies/{policy}/checks/
- **`tactical-rmm-cli automation automation-policies-9`** - GET /automation/policies/{policy}/tasks/
- **`tactical-rmm-cli automation automation-tasks`** - GET /automation/tasks/{task}/run/
- **`tactical-rmm-cli automation automation-tasks-2`** - POST /automation/tasks/{task}/run/
- **`tactical-rmm-cli automation automation-tasks-3`** - GET /automation/tasks/{task}/status/
- **`tactical-rmm-cli automation automation-tasks-4`** - POST /automation/tasks/{task}/status/

### autotasks

Manage autotasks

- **`tactical-rmm-cli autotasks autotasks`** - GET /autotasks/
- **`tactical-rmm-cli autotasks autotasks-endpoint`** - POST /autotasks/
- **`tactical-rmm-cli autotasks autotasks-pk`** - DELETE /autotasks/{pk}/
- **`tactical-rmm-cli autotasks autotasks-pk-2`** - GET /autotasks/{pk}/
- **`tactical-rmm-cli autotasks autotasks-pk-3`** - PUT /autotasks/{pk}/

### checks

Manage checks

- **`tactical-rmm-cli checks checks`** - GET /checks/
- **`tactical-rmm-cli checks checks-endpoint`** - POST /checks/
- **`tactical-rmm-cli checks checks-pk`** - DELETE /checks/{pk}/
- **`tactical-rmm-cli checks checks-pk-2`** - GET /checks/{pk}/
- **`tactical-rmm-cli checks checks-pk-3`** - PUT /checks/{pk}/

### clients

Manage clients

- **`tactical-rmm-cli clients clients`** - GET /clients/
- **`tactical-rmm-cli clients clients-deployments`** - DELETE /clients/deployments/
- **`tactical-rmm-cli clients clients-deployments-2`** - GET /clients/deployments/
- **`tactical-rmm-cli clients clients-deployments-3`** - POST /clients/deployments/
- **`tactical-rmm-cli clients clients-deployments-4`** - DELETE /clients/deployments/{pk}/
- **`tactical-rmm-cli clients clients-deployments-5`** - GET /clients/deployments/{pk}/
- **`tactical-rmm-cli clients clients-deployments-6`** - POST /clients/deployments/{pk}/
- **`tactical-rmm-cli clients clients-endpoint`** - POST /clients/
- **`tactical-rmm-cli clients clients-pk`** - DELETE /clients/{pk}/
- **`tactical-rmm-cli clients clients-pk-2`** - GET /clients/{pk}/
- **`tactical-rmm-cli clients clients-pk-3`** - PUT /clients/{pk}/
- **`tactical-rmm-cli clients clients-sites`** - GET /clients/sites/
- **`tactical-rmm-cli clients clients-sites-2`** - POST /clients/sites/
- **`tactical-rmm-cli clients clients-sites-3`** - DELETE /clients/sites/{pk}/
- **`tactical-rmm-cli clients clients-sites-4`** - GET /clients/sites/{pk}/
- **`tactical-rmm-cli clients clients-sites-5`** - PUT /clients/sites/{pk}/

### core

Manage core

- **`tactical-rmm-cli core core`** - GET /core/clearcache/
- **`tactical-rmm-cli core core-codesign`** - DELETE /core/codesign/
- **`tactical-rmm-cli core core-codesign-2`** - GET /core/codesign/
- **`tactical-rmm-cli core core-codesign-3`** - PATCH /core/codesign/
- **`tactical-rmm-cli core core-codesign-4`** - POST /core/codesign/
- **`tactical-rmm-cli core core-customfields`** - GET /core/customfields/
- **`tactical-rmm-cli core core-customfields-2`** - PATCH /core/customfields/
- **`tactical-rmm-cli core core-customfields-3`** - POST /core/customfields/
- **`tactical-rmm-cli core core-customfields-4`** - DELETE /core/customfields/{pk}/
- **`tactical-rmm-cli core core-customfields-5`** - GET /core/customfields/{pk}/
- **`tactical-rmm-cli core core-customfields-6`** - PUT /core/customfields/{pk}/
- **`tactical-rmm-cli core core-dashinfo`** - GET /core/dashinfo/
- **`tactical-rmm-cli core core-emailtest`** - GET /core/emailtest/
- **`tactical-rmm-cli core core-keystore`** - GET /core/keystore/
- **`tactical-rmm-cli core core-keystore-2`** - POST /core/keystore/
- **`tactical-rmm-cli core core-keystore-3`** - DELETE /core/keystore/{pk}/
- **`tactical-rmm-cli core core-keystore-4`** - PUT /core/keystore/{pk}/
- **`tactical-rmm-cli core core-openai`** - POST /core/openai/generate/
- **`tactical-rmm-cli core core-schedules`** - GET /core/schedules/
- **`tactical-rmm-cli core core-schedules-2`** - POST /core/schedules/
- **`tactical-rmm-cli core core-schedules-3`** - DELETE /core/schedules/{pk}/
- **`tactical-rmm-cli core core-schedules-4`** - PUT /core/schedules/{pk}/
- **`tactical-rmm-cli core core-servermaintenance`** - GET /core/servermaintenance/
- **`tactical-rmm-cli core core-settings`** - GET /core/settings/
- **`tactical-rmm-cli core core-settings-2`** - PUT /core/settings/
- **`tactical-rmm-cli core core-smstest`** - POST /core/smstest/
- **`tactical-rmm-cli core core-status`** - GET /core/status/
- **`tactical-rmm-cli core core-urlaction`** - GET /core/urlaction/
- **`tactical-rmm-cli core core-urlaction-2`** - POST /core/urlaction/
- **`tactical-rmm-cli core core-urlaction-3`** - PATCH /core/urlaction/run/
- **`tactical-rmm-cli core core-urlaction-4`** - DELETE /core/urlaction/{pk}/
- **`tactical-rmm-cli core core-urlaction-5`** - PUT /core/urlaction/{pk}/
- **`tactical-rmm-cli core core-urlaction-6`** - POST /core/urlaction/run/test/
- **`tactical-rmm-cli core core-v2`** - GET /core/v2/status/
- **`tactical-rmm-cli core core-version`** - GET /core/version/
- **`tactical-rmm-cli core core-webtermperms`** - GET /core/webtermperms/

### logs

Manage logs

- **`tactical-rmm-cli logs logs`** - PATCH /logs/audit/
- **`tactical-rmm-cli logs logs-debug`** - PATCH /logs/debug/
- **`tactical-rmm-cli logs logs-pendingactions`** - DELETE /logs/pendingactions/
- **`tactical-rmm-cli logs logs-pendingactions-2`** - GET /logs/pendingactions/
- **`tactical-rmm-cli logs logs-pendingactions-3`** - DELETE /logs/pendingactions/{pk}/
- **`tactical-rmm-cli logs logs-pendingactions-4`** - GET /logs/pendingactions/{pk}/

### scripts

Manage scripts

- **`tactical-rmm-cli scripts scripts`** - GET /scripts/
- **`tactical-rmm-cli scripts scripts-endpoint`** - POST /scripts/
- **`tactical-rmm-cli scripts scripts-pk`** - DELETE /scripts/{pk}/
- **`tactical-rmm-cli scripts scripts-pk-2`** - GET /scripts/{pk}/
- **`tactical-rmm-cli scripts scripts-pk-3`** - PUT /scripts/{pk}/
- **`tactical-rmm-cli scripts scripts-snippets`** - GET /scripts/snippets/
- **`tactical-rmm-cli scripts scripts-snippets-2`** - POST /scripts/snippets/
- **`tactical-rmm-cli scripts scripts-snippets-3`** - DELETE /scripts/snippets/{pk}/
- **`tactical-rmm-cli scripts scripts-snippets-4`** - GET /scripts/snippets/{pk}/
- **`tactical-rmm-cli scripts scripts-snippets-5`** - PUT /scripts/snippets/{pk}/

### services

Manage services

- **`tactical-rmm-cli services services`** - GET /services/{agent_id}/
- **`tactical-rmm-cli services services-agentid`** - GET /services/{agent_id}/{svcname}/
- **`tactical-rmm-cli services services-agentid-2`** - POST /services/{agent_id}/{svcname}/
- **`tactical-rmm-cli services services-agentid-3`** - PUT /services/{agent_id}/{svcname}/

### software

Manage software

- **`tactical-rmm-cli software software`** - GET /software/
- **`tactical-rmm-cli software software-agentid`** - GET /software/{agent_id}/
- **`tactical-rmm-cli software software-agentid-2`** - POST /software/{agent_id}/
- **`tactical-rmm-cli software software-agentid-3`** - PUT /software/{agent_id}/
- **`tactical-rmm-cli software software-chocos`** - GET /software/chocos/
- **`tactical-rmm-cli software software-endpoint`** - POST /software/
- **`tactical-rmm-cli software software-endpoint-2`** - PUT /software/

### winupdate

Manage winupdate

- **`tactical-rmm-cli winupdate winupdate`** - GET /winupdate/{agent_id}/
- **`tactical-rmm-cli winupdate winupdate-pk`** - PUT /winupdate/{pk}/


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
tactical-rmm-cli accounts accounts

# JSON for scripting and agents
tactical-rmm-cli accounts accounts --json

# Filter to specific fields
tactical-rmm-cli accounts accounts --json --select id,name,status

# Dry run  -  show the request without sending
tactical-rmm-cli accounts accounts --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
tactical-rmm-cli accounts accounts --agent
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
tactical-rmm-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/tactical-rmm-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `TRMM_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `tactical-rmm-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `tactical-rmm-cli doctor` to check credentials
- Verify the environment variable is set: `echo $TRMM_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized**  -  Set TRMM_API_KEY to a valid key and confirm the X-API-KEY header is sent.
- **Connection refused / DNS error**  -  Point at your API host (api.<domain>), not the web UI host; the API is a separate subdomain.
- **Empty results from a fleet command**  -  Run 'tactical-rmm-cli sync' first; cross-entity commands read the local store.
