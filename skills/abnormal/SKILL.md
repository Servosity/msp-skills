---
name: abnormal
description: "The full Abnormal Security REST API as an agent-ready CLI  -  with a local threat store, ranked SOC triage, and one-shot reporting no SOAR pack offers. Trigger phrases: `triage abnormal threats`, `remediate a phishing campaign in abnormal`, `abnormal email threat report`, `check account takeover risk for an employee`, `vendor email compromise check`, `use abnormal security`, `run abnormal-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Abnormal Security"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - abnormal-cli
    install:
      - kind: go
        bins: [abnormal-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/abnormal/cmd/abnormal-cli
---

# Abnormal Security  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `abnormal-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install abnormal --cli-only
   ```
2. Verify: `abnormal-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/abnormal/cmd/abnormal-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Every threat, case, vendor, employee, and dashboard operation from Abnormal's official API, plus a synced SQLite store that powers a ranked `triage` queue, joined `employee-risk` and `vendor-risk` investigation views, blocking `remediate-watch` confirmation, and a consolidated `report-snapshot` for client reporting. The incumbent integrations live inside SOAR and SIEM platforms; this runs in your terminal and your agents.

## When to Use This CLI

Use this CLI for SOC work against an Abnormal Security tenant: triaging the email threat feed, confirming remediations, investigating account-takeover cases and vendor email compromise, pulling employee identity and login context, and producing security reports from the dashboard aggregations. It is the right choice whenever an agent needs Abnormal threat or case data in structured form, or needs to act on a threat and verify the action completed.

## Anti-triggers

Do not use this CLI for:
- Quarantining or purging mail directly inside Microsoft 365 mailboxes  -  Abnormal remediates through its own actions; use Graph or Defender tooling for raw mailbox surgery
- Sending or composing email
- Changing Abnormal detection policies or integration settings  -  those are portal-only and not exposed by the REST API
- Continuous high-volume SIEM ingestion  -  use Abnormal's native Splunk/Elastic integrations for streaming pipelines

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### SOC triage that compounds
- **`triage`**  -  Surface the newest, highest-severity, still-unremediated threats first so an analyst starts the shift on what actually matters.

  _Reach for this when asked what email threats need attention now  -  it returns a ranked queue instead of a flat list._

  ```bash
  abnormal-cli triage --since 24h --top 20 --agent
  ```
- **`remediate-watch`**  -  Remediate a threat or case and block until Abnormal reports the action reached a terminal state, with a typed exit code and receipt.

  _Use this when a remediation must be confirmed done, not just submitted  -  exit code 0 means the action completed._

  ```bash
  abnormal-cli remediate-watch "threat" 184712ab-6d8b-47b3-89d7-a314efef23ff --timeout 5m
  ```

### Reporting and onboarding
- **`report-snapshot`**  -  Pull the key dashboard aggregations  -  attacks seen, attacks stopped, impersonation breakdowns, trending attacks  -  into one client-ready table or CSV.

  _Use this to build a weekly or QBR security report without screenshotting dashboard panels._

  ```bash
  abnormal-cli report-snapshot --since 30d --csv
  ```
- **`smoke`**  -  Verify token, base URL (US/EU), and IP allowlist reachability using Abnormal's vendor-supplied test payloads  -  without touching real tenant data.

  _Run this first when onboarding a tenant or rotating a token to prove connectivity before real queries._

  ```bash
  abnormal-cli smoke
  ```

### Investigation joins
- **`employee-risk`**  -  One account-takeover risk picture per employee: profile, Genome identity analysis, 30-day login pattern, and open cases naming them.

  _Use this when investigating a suspected account takeover to scope identity and blast radius in one call._

  ```bash
  abnormal-cli employee-risk vip@example.com --agent
  ```
- **`vendor-risk`**  -  One vendor-email-compromise picture per vendor: details, recent activity, and open vendor cases.

  _Use this when a vendor's emails look suspicious to see their whole relationship and case history at once._

  ```bash
  abnormal-cli vendor-risk acme-supplies.com --agent
  ```

## Command Reference

**abuse-mailbox**  -  Manage abuse mailbox

- `abnormal-cli abuse-mailbox`  -  Get a list of messages submitted to AI Security Mailbox (formerly known as Abuse Mailbox) that were not analyzed.

**abusecampaigns**  -  Manage abusecampaigns

- `abnormal-cli abusecampaigns retrieve`  -  Get a list of campaigns submitted to AI Security Mailbox (formerly known as Abuse Mailbox)
- `abnormal-cli abusecampaigns retrieve-2`  -  Get details of an abuse campaign

**aggregations**  -  Manage aggregations

- `abnormal-cli aggregations attack-frequency-retrieve`  -  Retrieve the frequency of specific attack types for a given period.
- `abnormal-cli aggregations attack-stopped-retrieve`  -  Retrieve aggregated counts of distinct attack types that were successfully stopped
- `abnormal-cli aggregations attack-strategy-breakdown-retrieve`  -  Retrieve the breakdown of attacks based on their strategy.
- `abnormal-cli aggregations attack-vector-breakdown-retrieve`  -  Retrieve the breakdown of attacks based on their vectors.
- `abnormal-cli aggregations attacker-origin-retrieve`  -  Retrieve the origin countries of attackers for a given period.
- `abnormal-cli aggregations dashboard-summary-retrieve`  -  Retrieve an aggregated summary of multiple security data points for the dashboard.
- `abnormal-cli aggregations most-impersonated-employee-non-vip-retrieve`  -  Retrieve the most impersonated non-VIP employees for a specified period.
- `abnormal-cli aggregations most-impersonated-employee-retrieve`  -  Retrieve the most impersonated employees for a specified period.
- `abnormal-cli aggregations most-impersonated-employee-vip-retrieve`  -  Retrieve the most impersonated VIP employees for a specified period.
- `abnormal-cli aggregations most-impersonated-vendor-retrieve`  -  Retrieve a list of the most impersonated vendors in attacks.
- `abnormal-cli aggregations recipient-employees-non-vip-retrieve`  -  Retrieve a list of the non-VIP employees who were recipients of attacks, based on their job titles.
- `abnormal-cli aggregations recipient-employees-retrieve`  -  Retrieve a list of the employees who were recipients of attacks, based on their job titles.
- `abnormal-cli aggregations recipient-employees-vip-retrieve`  -  Retrieve a list of the VIP employees who were recipients of attacks, based on their job titles.
- `abnormal-cli aggregations sender-impersonation-breakdown-retrieve`  -  Retrieve a breakdown of attacks based on sender impersonation.
- `abnormal-cli aggregations trending-attacks-retrieve`  -  Retrieve the list of trending attacks for a specified period.

**api_resources**  -  Manage api resources

- `abnormal-cli api-resources resources-actions-create`  -  Execute a specific action on a resource (refresh or validate). Returns 202 Accepted with action ID for tracking.
- `abnormal-cli api-resources resources-create-create`  -  Create a new resource with the specified name and optional description. Returns 201 Created with resource ID.
- `abnormal-cli api-resources resources-retrieve`  -  Retrieve a paginated list of resources with optional filtering using pageSize and pageNumber query parameters.
- `abnormal-cli api-resources resources-retrieve-2`  -  Retrieve detailed information about a specific resource by its UUID.
- `abnormal-cli api-resources resources-update-partial-update`  -  Partially update an existing resource's fields (PATCH). Provide only the fields that need updating.

**auditlogs**  -  Manage auditlogs

- `abnormal-cli auditlogs`  -  Gets a list of Audit Logs for Portal

**cases**  -  APIs to manage Abnormal Cases

- `abnormal-cli cases create`  -  Account Takeover license is required to call this endpoint. Use this to update the status of an abnormal case.
- `abnormal-cli cases retrieve`  -  Get a list of Abnormal cases identified by Abnormal Security
- `abnormal-cli cases retrieve-2`  -  Account Takeover license is required to call this endpoint.

**detection360**  -  Manage detection360

- `abnormal-cli detection360 reports-create`  -  Use this to report a detection misclassification judgement by Abnormal Security.
- `abnormal-cli detection360 reports-retrieve`  -  Get a list of Detection 360 reports that you have submitted and view corresponding details for each case

**email_search**  -  Manage email search

- `abnormal-cli email-search search-activities-retrieve`  -  List activity logs for search and remediation operations. Optionally filter by tenant_ids query parameter (e.g., ?
- `abnormal-cli email-search search-activities-status-retrieve`  -  Get detailed status of a specific activity including remediation results.
- `abnormal-cli email-search search-create`  -  Search for email messages across Abnormal and Quarantine sources. Optionally filter by tenant_ids in the request body.
- `abnormal-cli email-search search-messages-attachments-download-retrieve`  -  Download an email attachment for a given message.
- `abnormal-cli email-search search-messages-eml-retrieve`  -  Download the EML file for a specific message by cloud_message_id. Returns the EML file content as message/rfc822 format.
- `abnormal-cli email-search search-remediate-create`  -  Remediate email messages by deleting, moving, or submitting them for review.

**employee**  -  Manage employee

- `abnormal-cli employee <email_address>`  -  Get employee information

**messages**  -  API to manage message details


**roles**  -  API to retrieve roles from RBAC system

- `abnormal-cli roles`  -  Fetch all roles for an account from RBAC system.

**security-settings**  -  API to retrieve security settings including session timeout configuration

- `abnormal-cli security-settings`  -  Fetch security settings for an account.

**soar**  -  Manage soar

- `abnormal-cli soar`  -  Fetch all API tokens for the authenticated customer from the Go Token Management Service.

**spm-v2**  -  Manage spm v2

- `abnormal-cli spm-v2 posture-catalog-retrieve`  -  Get posture catalog containing all available abnormal supported postures
- `abnormal-cli spm-v2 postures-query-create`  -  Get a list of all tenant postures
- `abnormal-cli spm-v2 postures-retrieve`  -  Get detailed information about a specific security posture evaluation
- `abnormal-cli spm-v2 postures-timeline-retrieve`  -  Get timeline of events for a specific security posture
- `abnormal-cli spm-v2 reports-summary-retrieve`  -  Get summary report for all postures
- `abnormal-cli spm-v2 workflow-logs-raw-json-retrieve`  -  Get raw JSON for a workflow log

**threats**  -  APIs to manage threats notified in the Abnormal Threat Log

- `abnormal-cli threats create`  -  Use this to remediate or unremediate a threat.
- `abnormal-cli threats retrieve`  -  Get a list of threats
- `abnormal-cli threats retrieve-2`  -  Get details of a threat

**threats-export**  -  Manage threats export

- `abnormal-cli threats-export`  -  Download data from Threat Log in .csv format

**url-rewrite**  -  Manage url rewrite

- `abnormal-cli url-rewrite`  -  Retrieve paginated click and clickthrough events for URL rewrites.

**users**  -  API to retrieve users from RBAC system

- `abnormal-cli users`  -  Retrieves users for an account from the RBAC user management system.

**vendor-cases**  -  Manage vendor cases

- `abnormal-cli vendor-cases retrieve`  -  Get a list of vendor cases.
- `abnormal-cli vendor-cases retrieve-2`  -  Get details of a vendor case

**vendors**  -  API to manage Vendorbase and threats from Vendors

- `abnormal-cli vendors`  -  Get a list of vendors your organization has interacted with


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
abnormal-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Morning triage

```bash
abnormal-cli triage --since 24h --top 20 --agent --select items.threatId,items.attackType,items.score
```

Ranked unremediated threats from the local store, narrowed to the three fields an agent needs to decide what to open first.

### Remediate and confirm

```bash
abnormal-cli remediate-watch "threat" 184712ab-6d8b-47b3-89d7-a314efef23ff --timeout 5m
```

Submits the remediation and blocks until Abnormal reports a terminal action state  -  exit 0 means confirmed done.

### Account-takeover scoping

```bash
abnormal-cli employee-risk vip@example.com --agent
```

Profile, Genome identity analysis, 30-day logins, and open cases for one employee in a single structured payload.

### Client report numbers

```bash
abnormal-cli report-snapshot --since 30d --csv
```

The dashboard aggregations consolidated into CSV for pasting straight into a client report.

### Vendor compromise check

```bash
abnormal-cli vendor-risk acme-supplies.com --agent
```

Vendor details, recent activity, and open vendor cases joined into one investigation view.

## Auth Setup

Mint a REST API token in the Abnormal portal under Settings → Integrations → Abnormal REST API, then export it as `ABNORMAL_API_TOKEN`. Abnormal enforces IP allowlisting on the integration: add your egress IP in the same portal screen or every call returns 403. EU tenants must point the CLI at the EU host: `export ABNORMAL_BASE_URL=https://eu.rest.abnormalsecurity.com/v1`. You can also store the token with `abnormal-cli auth set-token <token>` and inspect it with `abnormal-cli auth status`.

Run `abnormal-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  abnormal-cli abuse-mailbox --agent --select id,name,status
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
abnormal-cli feedback "the --since flag is inclusive but docs say exclusive"
abnormal-cli feedback --stdin < notes.txt
abnormal-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/abnormal-cli/feedback.jsonl`. They are never POSTed unless `ABNORMAL_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `ABNORMAL_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
abnormal-cli profile save briefing --json
abnormal-cli --profile briefing abuse-mailbox
abnormal-cli profile list --json
abnormal-cli profile show briefing
abnormal-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `abnormal-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/abnormal/cmd/abnormal-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add abnormal-mcp -- abnormal-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which abnormal-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   abnormal-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `abnormal-cli <command> --help`.
