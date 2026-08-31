# ThreatLocker CLI

**Every ThreatLocker Portal API feature, plus the write operations the read-only tools lack and a cross-tenant offline store no other ThreatLocker tool has.**

A single CLI for MSPs running ThreatLocker across many customer tenants. It matches the full read surface of the incumbent MCP server, adds the writes nobody shipped (approve requests, toggle maintenance, push policy), and mirrors every entity into a local SQLite database so you can triage approvals, audit drift, and device health across ALL tenants at once  -  something the per-tenant API forces you to do one header-swap at a time.

Created by [@dstevens](https://github.com/dstevens) (Damien Stevens).
Contributors: [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `threatlocker-cli` and `threatlocker-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatlocker/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatlocker/install.ps1 | iex
   ```
3. Verify: `threatlocker-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=threatlocker). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/threatlocker --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/threatlocker --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `threatlocker-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the threatlocker skill from https://github.com/Servosity/msp-skills/tree/main/skills/threatlocker. The skill defines how its required CLI (`threatlocker-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=threatlocker).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `THREATLOCKER_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

> **Interim note:** a `.mcpb` bundle built before the [#287](https://github.com/Servosity/msp-skills/issues/287) fix does not launch. Its `manifest.json` runs `${__dirname}/bin/threatlocker-pp-mcp`, but the archive stores the binaries under platform-suffixed names (`bin/threatlocker-pp-mcp-darwin-arm64` and four siblings), so Claude Desktop finds nothing at that path. Check the bundle you downloaded with `unzip -l <file>.mcpb | grep bin/`: if `bin/threatlocker-pp-mcp` is not listed, use the installer above and the manual JSON config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatlocker/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatlocker/install.ps1 | iex            # Windows
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

Drain the overnight backlog ranked by age with duplicate hashes collapsed, then batch-approve the trusted ones.

### Nightly audit archive before the 31-day cliff

```bash
threatlocker-cli audit export --all-tenants --since 2026-04-01 --csv > audit-archive.csv
```

Persist Unified Audit beyond ThreatLocker's retention window for SIEM and compliance.

### Who disabled protection this week

```bash
threatlocker-cli audit drift --since 7d --all-tenants --agent
```

One ranked table of protection-off / policy-change / maintenance events across every customer.

### Dark-agent health sweep

```bash
threatlocker-cli devices health --all-tenants --agent --select organizationName,computerName,healthClass,lastCheckin
```

Classify every endpoint healthy/offline/stale/isolated across all tenants in one pass.

### Diagnose a broken automation

```bash
threatlocker-cli doctor --agent
```

Pinpoint whether a 401 is an expired token, a missing org header, or Old-API mode.

## Usage

Run `threatlocker-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `THREATLOCKER_CONFIG_DIR`, `THREATLOCKER_DATA_DIR`, `THREATLOCKER_STATE_DIR`, or `THREATLOCKER_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `THREATLOCKER_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export THREATLOCKER_HOME=/srv/threatlocker
threatlocker-cli doctor
```

Under `THREATLOCKER_HOME=/srv/threatlocker`, the four dirs resolve to `/srv/threatlocker/config`, `/srv/threatlocker/data`, `/srv/threatlocker/state`, and `/srv/threatlocker/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "threatlocker": {
      "command": "threatlocker-mcp",
      "env": {
        "THREATLOCKER_HOME": "/srv/threatlocker"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `THREATLOCKER_DATA_DIR` overrides an explicit `--home` for that kind. Use `THREATLOCKER_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `THREATLOCKER_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `threatlocker-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

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


### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`threatlocker-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`threatlocker-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`threatlocker-cli learnings list`** - Inspect taught rows
- **`threatlocker-cli learnings forget <query>`** - Undo a teach
- **`threatlocker-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`threatlocker-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`threatlocker-cli teach-pattern`** - Install a query/resource template up front
- **`threatlocker-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `THREATLOCKER_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `threatlocker-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
threatlocker-cli application-files --application-id 550e8400-e29b-41d4-a716-446655440000

# JSON for scripting and agents
threatlocker-cli application-files --application-id 550e8400-e29b-41d4-a716-446655440000 --json
# Filter to specific fields
threatlocker-cli application-files --application-id 550e8400-e29b-41d4-a716-446655440000 --json --select applicationFileId,applicationId,fullPath

# Dry run  -  show the request without sending
threatlocker-cli application-files --application-id 550e8400-e29b-41d4-a716-446655440000 --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
threatlocker-cli application-files --application-id 550e8400-e29b-41d4-a716-446655440000 --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
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

Run `threatlocker-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/threatlocker-cli/config.toml`; `--home`, `THREATLOCKER_HOME`, and per-kind env vars can relocate it.

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

---

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**threatlocker-mcp-server**](https://github.com/BigfootBytes/threatlocker-mcp-server)  -  TypeScript (1 stars)
- [**DynamicIT/ThreatLocker**](https://github.com/DynamicIT/ThreatLocker)  -  PowerShell

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
