# ThreatLocker CLI

**Every ThreatLocker Portal API feature, plus the write operations the read-only tools lack and a cross-tenant offline store no other ThreatLocker tool has.**

A single CLI for MSPs running ThreatLocker across many customer tenants. It matches the full read surface of the incumbent MCP server, adds the writes nobody shipped (approve requests, toggle maintenance, push policy), and mirrors every entity into a local SQLite database so you can triage approvals, audit drift, and device health across ALL tenants at once  -  something the per-tenant API forces you to do one header-swap at a time.

## Install

The recommended path installs both the `threatlocker-cli` binary and the `pp-threatlocker` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install threatlocker
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install threatlocker --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install threatlocker --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install threatlocker --agent claude-code
npx -y @mvanhorn/printing-press-library install threatlocker --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/threatlocker/cmd/threatlocker-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/threatlocker-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install threatlocker --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-threatlocker --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-threatlocker --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install threatlocker --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/threatlocker-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `THREATLOCKER_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/threatlocker/cmd/threatlocker-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "threatlocker": {
      "command": "threatlocker-mcp",
      "env": {
        "THREATLOCKER_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Auth is a raw API token in the Authorization header (NO 'Bearer' prefix)  -  a 64-character lowercase hex string created in the portal under Administrators > API Users > Generate API Token. Most calls also need a ManagedOrganizationId header (your tenant GUID); set THREATLOCKER_ORG_ID or pass --org. Tokens renew on each use and silently expire when idle, so run `doctor` if you hit a 401.

## Quick Start

```bash
# verify your token, org header, and API mode before anything else
threatlocker-cli doctor

# list your managed customer tenants and their GUIDs
threatlocker-cli organizations list --agent

# see pending approval requests across all tenants
threatlocker-cli approvals list --child-orgs --agent

# mirror entities into the local store so cross-tenant commands work offline
threatlocker-cli sync

# the ranked, hash-grouped cross-tenant approval queue
threatlocker-cli approvals triage --all-tenants --agent

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Cross-tenant intelligence
- **`approvals triage`**  -  One ranked queue of every pending application approval across all your managed customer tenants, grouped by file hash so duplicate requests collapse into one row.

  _Reach for this to clear the morning approval backlog across an entire MSP book without swapping tenant context request-by-request._

  ```bash
  threatlocker-cli approvals triage --all-tenants --agent
  ```
- **`audit drift`**  -  One ranked table of security-relevant changes  -  protection disabled, policy changed, maintenance toggled  -  across every tenant in a time window.

  _Use this for the weekly compliance sweep or right after a customer reports something changed unexpectedly._

  ```bash
  threatlocker-cli audit drift --since 7d --all-tenants --agent
  ```
- **`devices health`**  -  Joins computers, online-devices, and last-checkin data to classify every endpoint healthy / offline / stale / isolated, rolled up per tenant.

  _Reach for this for the daily 'which agents are dark across all customers' sweep and post-patch verification._

  ```bash
  threatlocker-cli devices health --all-tenants --agent
  ```
- **`applications hunt`**  -  Locate a specific file (by hash, certificate, or path) across every tenant and endpoint in one offline query  -  present, approved, or pending.

  _Reach for this during incident response to answer 'where does this binary live across my whole book' without swapping tenant context per search._

  ```bash
  threatlocker-cli applications hunt --hash 3a7bd3e2360a3d29eea436fcfb7e44c735d117c42d1c1835420b6b9942dd4f1b --agent
  ```

### MSP write operations
- **`approvals approve-batch`**  -  Approve the same file (by SHA256) across every tenant where it is pending, in one command, with a dry-run plan first.

  _Use this when one trusted updater is blocked everywhere  -  approve it once instead of clicking through 30 portals._

  ```bash
  threatlocker-cli approvals approve-batch --hash e3b0c44298fc1c149afbf4c8996fb924... --all-tenants --dry-run
  ```

### Audit & compliance
- **`audit export`**  -  Export the Unified Audit log per-tenant or across all tenants to JSONL/CSV and persist it locally, keeping evidence past ThreatLocker's 31-day retention cliff.

  _Run this on a schedule so compliance evidence and incident timelines survive the retention window._

  ```bash
  threatlocker-cli audit export --all-tenants --since 2026-04-01 --agent
  ```
- **`audit retention-check`**  -  Reports, per tenant, the oldest audit row you have versus the 31-day cliff and how stale your last export is  -  flagging tenants about to lose evidence.

  _Pick this to catch a broken export before the data it should have captured ages off the 31-day window forever._

  ```bash
  threatlocker-cli audit retention-check --agent
  ```

### Auth resilience
- **`doctor`**  -  Diagnoses the #1 ThreatLocker integration pain: validates the raw 64-hex token format, the no-Bearer Authorization header, the ManagedOrganizationId header, New-vs-Old API mode, pings an authenticated endpoint, and maps a 401 to its exact likely cause.

  _Run this first whenever a script starts returning 401  -  it tells you whether the token expired, the org header is missing, or you're on the deprecated API mode._

  ```bash
  threatlocker-cli doctor --agent
  ```

## Recipes


### Morning cross-tenant approval sweep

```bash
threatlocker-cli approvals triage --all-tenants --agent --select organizationName,fileName,hash,ageHours,duplicateCount
```

Drain the overnight backlog ranked by age with duplicate hashes collapsed, then batch-approve the trusted ones. Requires a recent sync --resources approvals.

### Nightly audit archive before the 31-day cliff

```bash
threatlocker-cli audit export --all-tenants --since 2026-04-01 --csv > audit-archive.csv
```

Persist Unified Audit beyond ThreatLocker's retention window for SIEM and compliance. Requires a recent sync --resources audit  -  the export materializes from the local archive.

### Who disabled protection this week

```bash
threatlocker-cli audit drift --since 7d --all-tenants --agent
```

One ranked table of protection-off / policy-change / maintenance events across every customer. Requires a recent sync --resources audit,system-audit.

### Dark-agent health sweep

```bash
threatlocker-cli devices health --all-tenants --agent --select organizationName,computerName,healthClass,lastCheckin
```

Classify every endpoint healthy/offline/stale/isolated across all tenants in one pass. Requires a recent sync --resources computers,online-devices.

### Diagnose a broken automation

```bash
threatlocker-cli doctor --agent
```

Pinpoint whether a 401 is an expired token, a missing org header, or Old-API mode.

## Usage

Run `threatlocker-cli --help` for the full command reference and flag list.

## Commands

### application-files

File rules belonging to an application

- **`threatlocker-cli application-files`** - List the file rules within an application (paginated)

### applications

Application definitions (custom + built-in) and policies' targets

- **`threatlocker-cli applications create`** - Create a custom application definition
- **`threatlocker-cli applications get`** - Get a single application by id
- **`threatlocker-cli applications match`** - Match a file (hash/path/cert) to existing applications  -  used in the approval flow
- **`threatlocker-cli applications research`** - ThreatLocker security research details (risk ratings, categories, remediation)
- **`threatlocker-cli applications search`** - Search applications (paginated). searchBy: app/full/process/hash/cert/created/categories/countries.
- **`threatlocker-cli applications update`** - Update an application's name/description

### approvals

Application-control approval requests (list, inspect, approve)

- **`threatlocker-cli approvals approve`** - Approve (permit) an application approval request, creating/extending a permit policy. policyLevel: org/group/computer.
- **`threatlocker-cli approvals count`** - Count of pending approval requests
- **`threatlocker-cli approvals get`** - Get a single approval request
- **`threatlocker-cli approvals list`** - List approval requests. statusId 1=Pending,4=Approved,10=Ignored,13=Escalated. Use --child-orgs to span tenants.
- **`threatlocker-cli approvals permit-options`** - Get the permit options for an approval request (inputs to approve)
- **`threatlocker-cli approvals storage`** - Get storage-control approval request details

### audit

Unified Audit (ActionLog)  -  permit/deny events. Default retention 31 days.

- **`threatlocker-cli audit file-history`** - All audit events for a given file path
- **`threatlocker-cli audit get`** - Get a single audit entry by id
- **`threatlocker-cli audit search`** - Search the Unified Audit log. actionId 1=Permit,2=Deny,99=AnyDeny. Requires startDate/endDate.

### computer-groups

Computer groups

- **`threatlocker-cli computer-groups dropdown`** - Simple group dropdown (label/value)
- **`threatlocker-cli computer-groups list`** - List computer groups with nested computers

### computers

Manage and inspect protected computers/devices

- **`threatlocker-cli computers baseline-rescan`** - Restart Baseline (learning) on computers
- **`threatlocker-cli computers checkins`** - Connection/check-in history for a computer (paginated)
- **`threatlocker-cli computers delete`** - Delete/remove computers by id
- **`threatlocker-cli computers enable-protection`** - Enable Secured Mode (re-enable protection) on computers
- **`threatlocker-cli computers get`** - Get a single computer's detail by id
- **`threatlocker-cli computers install-info`** - Deployment/install info for adding new computers
- **`threatlocker-cli computers list`** - List/search computers (paginated). searchBy 1-5; orderBy e.g. computername.
- **`threatlocker-cli computers maintenance`** - Enable maintenance mode (disable protection) on computers for a window
- **`threatlocker-cli computers maintenance-update`** - Set/extend maintenance mode on a single computer
- **`threatlocker-cli computers move-org`** - Move computers to another organization (tenant)
- **`threatlocker-cli computers restart-service`** - Restart the ThreatLocker service on computers

### maintenance

Maintenance-mode history

- **`threatlocker-cli maintenance`** - Maintenance-mode history for a computer (paginated)

### network-policies

Network Control (network access) policies

- **`threatlocker-cli network-policies get`** - Get a single network access policy by id
- **`threatlocker-cli network-policies list`** - List network access policies (paginated)

### online-devices

Currently-online devices

- **`threatlocker-cli online-devices`** - List currently-online devices (paginated)

### organizations

Managed (child) organizations  -  MSP tenants

- **`threatlocker-cli organizations auth-key`** - Get the installation auth key for the current organization
- **`threatlocker-cli organizations for-move`** - List organizations available as computer-move targets
- **`threatlocker-cli organizations list`** - List child/managed organizations (paginated)

### policies

Application Control / Storage / Network policies

- **`threatlocker-cli policies copy`** - Copy policies from a source org/group to target org(s)  -  cross-tenant cloning
- **`threatlocker-cli policies create`** - Create a policy. policyActionId 1=Permit,2=Deny,6=Permit+Ringfence.
- **`threatlocker-cli policies delete`** - Delete policies by id
- **`threatlocker-cli policies deploy`** - Queue a policy deployment for an organization
- **`threatlocker-cli policies get`** - Get a single policy by id
- **`threatlocker-cli policies list-by-app`** - List policies that target an application (paginated)

### reports

Reports

- **`threatlocker-cli reports data`** - Fetch dynamic data for a report
- **`threatlocker-cli reports list`** - List report categories and their reports

### scheduled-actions

Scheduled agent actions

- **`threatlocker-cli scheduled-actions get`** - Get a single scheduled action by id
- **`threatlocker-cli scheduled-actions list`** - List scheduled agent actions
- **`threatlocker-cli scheduled-actions search`** - Search scheduled actions (paginated)

### storage-policies

Storage Control policies

- **`threatlocker-cli storage-policies get`** - Get a single storage policy by id
- **`threatlocker-cli storage-policies list`** - List storage policies (paginated)

### system-audit

Portal system audit (admin actions) + Health Center

- **`threatlocker-cli system-audit health-center`** - Health Center data for the last N days (1-365)
- **`threatlocker-cli system-audit search`** - Search portal admin/system audit entries. Requires startDate/endDate.

### tags

Tags

- **`threatlocker-cli tags dropdown`** - Tag dropdown options (label/value)
- **`threatlocker-cli tags get`** - Get a single tag (with its values) by id

### versions

ThreatLocker agent versions

- **`threatlocker-cli versions`** - List available agent versions (label/value/isEnabled/isDefault/osType)


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
threatlocker-cli application-files --application-id 550e8400-e29b-41d4-a716-446655440000

# JSON for scripting and agents
threatlocker-cli application-files --application-id 550e8400-e29b-41d4-a716-446655440000 --json

# Filter to specific fields
threatlocker-cli application-files --application-id 550e8400-e29b-41d4-a716-446655440000 --json --select id,name,status

# Dry run  -  show the request without sending
threatlocker-cli application-files --application-id 550e8400-e29b-41d4-a716-446655440000 --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
threatlocker-cli application-files --application-id 550e8400-e29b-41d4-a716-446655440000 --agent
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
threatlocker-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/threatlocker-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `THREATLOCKER_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `threatlocker-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `threatlocker-cli doctor` to check credentials
- Verify the environment variable is set: `echo $THREATLOCKER_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized from every call**  -  Run `threatlocker-cli doctor`; most often the sliding-expiry token died from inactivity or the ManagedOrganizationId header is missing.
- **Empty results but no error**  -  Confirm the ManagedOrganizationId / --org GUID is the tenant you mean; data calls are tenant-scoped.
- **Audit rows older than ~31 days are gone**  -  ThreatLocker retains ActionLog 31 days; run `audit export` on a schedule and query the local store for older data.
- **Endpoint deprecated / unexpected shape**  -  Ensure you are on the New API version; the Old API is deprecated and requires the ManagedOrganizationId header on the New one.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**threatlocker-mcp-server**](https://github.com/BigfootBytes/threatlocker-mcp-server)  -  TypeScript (1 stars)
- [**DynamicIT/ThreatLocker**](https://github.com/DynamicIT/ThreatLocker)  -  PowerShell

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
