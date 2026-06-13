---
name: threatlocker
description: "Every ThreatLocker Portal API feature, plus the write operations the read-only tools lack and a cross-tenant offline store no other ThreatLocker tool has. Trigger phrases: `triage threatlocker approvals`, `approve this hash across all tenants`, `export the threatlocker audit log`, `which threatlocker agents are offline`, `why is threatlocker returning 401`, `use threatlocker`, `run threatlocker`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "ThreatLocker"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - threatlocker-cli
    install:
      - kind: go
        bins: [threatlocker-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/threatlocker/cmd/threatlocker-cli
---

# ThreatLocker  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `threatlocker-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install threatlocker --cli-only
   ```
2. Verify: `threatlocker-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/threatlocker/cmd/threatlocker-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

A single CLI for MSPs running ThreatLocker across many customer tenants. It matches the full read surface of the incumbent MCP server, adds the writes nobody shipped (approve requests, toggle maintenance, push policy), and mirrors every entity into a local SQLite database so you can triage approvals, audit drift, and device health across ALL tenants at once  -  something the per-tenant API forces you to do one header-swap at a time.

## When to Use This CLI

Use this CLI when you operate ThreatLocker across multiple customer organizations and need to act, not just read  -  clearing approval backlogs at scale, exporting audit evidence before it ages off, finding unhealthy agents, or pushing policy across tenants. It is the right tool when the portal's one-tenant-at-a-time UI or the read-only MCP server is the bottleneck.

## Unique Capabilities

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

## Command Reference

**application-files**  -  File rules belonging to an application

- `threatlocker-cli application-files`  -  List the file rules within an application (paginated)

**applications**  -  Application definitions (custom + built-in) and policies' targets

- `threatlocker-cli applications create`  -  Create a custom application definition
- `threatlocker-cli applications get`  -  Get a single application by id
- `threatlocker-cli applications match`  -  Match a file (hash/path/cert) to existing applications  -  used in the approval flow
- `threatlocker-cli applications research`  -  ThreatLocker security research details (risk ratings, categories, remediation)
- `threatlocker-cli applications search`  -  Search applications (paginated). searchBy: app/full/process/hash/cert/created/categories/countries.
- `threatlocker-cli applications update`  -  Update an application's name/description

**approvals**  -  Application-control approval requests (list, inspect, approve)

- `threatlocker-cli approvals approve`  -  Approve (permit) an application approval request, creating/extending a permit policy. policyLevel: org/group/computer.
- `threatlocker-cli approvals count`  -  Count of pending approval requests
- `threatlocker-cli approvals get`  -  Get a single approval request
- `threatlocker-cli approvals list`  -  List approval requests. statusId 1=Pending,4=Approved,10=Ignored,13=Escalated. Use --child-orgs to span tenants.
- `threatlocker-cli approvals permit-options`  -  Get the permit options for an approval request (inputs to approve)
- `threatlocker-cli approvals storage`  -  Get storage-control approval request details

**audit**  -  Unified Audit (ActionLog)  -  permit/deny events. Default retention 31 days.

- `threatlocker-cli audit file-history`  -  All audit events for a given file path
- `threatlocker-cli audit get`  -  Get a single audit entry by id
- `threatlocker-cli audit search`  -  Search the Unified Audit log. actionId 1=Permit,2=Deny,99=AnyDeny. Requires startDate/endDate.

**computer-groups**  -  Computer groups

- `threatlocker-cli computer-groups dropdown`  -  Simple group dropdown (label/value)
- `threatlocker-cli computer-groups list`  -  List computer groups with nested computers

**computers**  -  Manage and inspect protected computers/devices

- `threatlocker-cli computers baseline-rescan`  -  Restart Baseline (learning) on computers
- `threatlocker-cli computers checkins`  -  Connection/check-in history for a computer (paginated)
- `threatlocker-cli computers delete`  -  Delete/remove computers by id
- `threatlocker-cli computers enable-protection`  -  Enable Secured Mode (re-enable protection) on computers
- `threatlocker-cli computers get`  -  Get a single computer's detail by id
- `threatlocker-cli computers install-info`  -  Deployment/install info for adding new computers
- `threatlocker-cli computers list`  -  List/search computers (paginated). searchBy 1-5; orderBy e.g. computername.
- `threatlocker-cli computers maintenance`  -  Enable maintenance mode (disable protection) on computers for a window
- `threatlocker-cli computers maintenance-update`  -  Set/extend maintenance mode on a single computer
- `threatlocker-cli computers move-org`  -  Move computers to another organization (tenant)
- `threatlocker-cli computers restart-service`  -  Restart the ThreatLocker service on computers

**maintenance**  -  Maintenance-mode history

- `threatlocker-cli maintenance`  -  Maintenance-mode history for a computer (paginated)

**network-policies**  -  Network Control (network access) policies

- `threatlocker-cli network-policies get`  -  Get a single network access policy by id
- `threatlocker-cli network-policies list`  -  List network access policies (paginated)

**online-devices**  -  Currently-online devices

- `threatlocker-cli online-devices`  -  List currently-online devices (paginated)

**organizations**  -  Managed (child) organizations  -  MSP tenants

- `threatlocker-cli organizations auth-key`  -  Get the installation auth key for the current organization
- `threatlocker-cli organizations for-move`  -  List organizations available as computer-move targets
- `threatlocker-cli organizations list`  -  List child/managed organizations (paginated)

**policies**  -  Application Control / Storage / Network policies

- `threatlocker-cli policies copy`  -  Copy policies from a source org/group to target org(s)  -  cross-tenant cloning
- `threatlocker-cli policies create`  -  Create a policy. policyActionId 1=Permit,2=Deny,6=Permit+Ringfence.
- `threatlocker-cli policies delete`  -  Delete policies by id
- `threatlocker-cli policies deploy`  -  Queue a policy deployment for an organization
- `threatlocker-cli policies get`  -  Get a single policy by id
- `threatlocker-cli policies list-by-app`  -  List policies that target an application (paginated)

**reports**  -  Reports

- `threatlocker-cli reports data`  -  Fetch dynamic data for a report
- `threatlocker-cli reports list`  -  List report categories and their reports

**scheduled-actions**  -  Scheduled agent actions

- `threatlocker-cli scheduled-actions get`  -  Get a single scheduled action by id
- `threatlocker-cli scheduled-actions list`  -  List scheduled agent actions
- `threatlocker-cli scheduled-actions search`  -  Search scheduled actions (paginated)

**storage-policies**  -  Storage Control policies

- `threatlocker-cli storage-policies get`  -  Get a single storage policy by id
- `threatlocker-cli storage-policies list`  -  List storage policies (paginated)

**system-audit**  -  Portal system audit (admin actions) + Health Center

- `threatlocker-cli system-audit health-center`  -  Health Center data for the last N days (1-365)
- `threatlocker-cli system-audit search`  -  Search portal admin/system audit entries. Requires startDate/endDate.

**tags**  -  Tags

- `threatlocker-cli tags dropdown`  -  Tag dropdown options (label/value)
- `threatlocker-cli tags get`  -  Get a single tag (with its values) by id

**versions**  -  ThreatLocker agent versions

- `threatlocker-cli versions`  -  List available agent versions (label/value/isEnabled/isDefault/osType)


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
threatlocker-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

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

## Auth Setup

Auth is a raw API token in the Authorization header (NO 'Bearer' prefix)  -  a 64-character lowercase hex string created in the portal under Administrators > API Users > Generate API Token. Most calls also need a ManagedOrganizationId header (your tenant GUID); set THREATLOCKER_ORG_ID or pass --org. Tokens renew on each use and silently expire when idle, so run `doctor` if you hit a 401.

Run `threatlocker-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  threatlocker-cli application-files --application-id 550e8400-e29b-41d4-a716-446655440000 --agent --select id,name,status
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set  -  piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
threatlocker-cli feedback "the --since flag is inclusive but docs say exclusive"
threatlocker-cli feedback --stdin < notes.txt
threatlocker-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/threatlocker-cli/feedback.jsonl`. They are never POSTed unless `THREATLOCKER_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `THREATLOCKER_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename) |
| `webhook:<url>` | POST the output body to the URL (`application/json` or `application/x-ndjson` when `--compact`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled agent calls the same command every run with the same configuration - HeyGen's "Beacon" pattern.

```
threatlocker-cli profile save briefing --json
threatlocker-cli --profile briefing application-files --application-id 550e8400-e29b-41d4-a716-446655440000
threatlocker-cli profile list --json
threatlocker-cli profile show briefing
threatlocker-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `threatlocker-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/threatlocker/cmd/threatlocker-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add threatlocker-mcp -- threatlocker-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which threatlocker-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   threatlocker-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `threatlocker-cli <command> --help`.
