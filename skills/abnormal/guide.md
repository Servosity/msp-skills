# Abnormal Security CLI

**The full Abnormal Security REST API as an agent-ready CLI  -  with a local threat store, ranked SOC triage, and one-shot reporting no SOAR pack offers.**

Every threat, case, vendor, employee, and dashboard operation from Abnormal's official API, plus a synced SQLite store that powers a ranked `triage` queue, joined `employee-risk` and `vendor-risk` investigation views, blocking `remediate-watch` confirmation, and a consolidated `report-snapshot` for client reporting. The incumbent integrations live inside SOAR and SIEM platforms; this runs in your terminal and your agents.

## Install

The recommended path installs both the `abnormal-cli` binary and the `pp-abnormal` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install abnormal
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install abnormal --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install abnormal --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install abnormal --agent claude-code
npx -y @mvanhorn/printing-press-library install abnormal --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/abnormal/cmd/abnormal-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/abnormal-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install abnormal --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-abnormal --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-abnormal --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install abnormal --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/abnormal-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `ABNORMAL_API_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/abnormal/cmd/abnormal-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "abnormal": {
      "command": "abnormal-mcp",
      "env": {
        "ABNORMAL_API_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Mint a REST API token in the Abnormal portal under Settings → Integrations → Abnormal REST API, then export it as `ABNORMAL_API_TOKEN`. Abnormal enforces IP allowlisting on the integration: add your egress IP in the same portal screen or every call returns 403. EU tenants must point the CLI at the EU host: `export ABNORMAL_BASE_URL=https://eu.rest.abnormalsecurity.com/v1`. You can also store the token with `abnormal-cli auth set-token <token>` and inspect it with `abnormal-cli auth status`.

## Quick Start

```bash
# Health check: config, token presence, and API reachability without mutating anything
abnormal-cli doctor --dry-run

# Prove token + base URL + IP allowlist using Abnormal's safe Mock-Data payloads
abnormal-cli smoke

# Pull the last week of threats and cases into the local SQLite store
abnormal-cli sync --resources threats,cases --since 7d

# Ranked queue of the newest, worst, still-unremediated threats
abnormal-cli triage --since 24h --top 20

# Consolidated dashboard numbers for a client-ready monthly report
abnormal-cli report-snapshot --since 30d

```

## Unique Features

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

## Usage

Run `abnormal-cli --help` for the full command reference and flag list.

## Commands

### abuse-mailbox

Manage abuse mailbox

- **`abnormal-cli abuse-mailbox`** - Get a list of messages submitted to AI Security Mailbox (formerly known as Abuse Mailbox) that were not analyzed.

### abusecampaigns

Manage abusecampaigns

- **`abnormal-cli abusecampaigns retrieve`** - Get a list of campaigns submitted to AI Security Mailbox (formerly known as Abuse Mailbox)
- **`abnormal-cli abusecampaigns retrieve-2`** - Get details of an abuse campaign

### aggregations

Manage aggregations

- **`abnormal-cli aggregations attack-frequency-retrieve`** - Retrieve the frequency of specific attack types for a given period.
- **`abnormal-cli aggregations attack-stopped-retrieve`** - Retrieve aggregated counts of distinct attack types that were successfully stopped, including current and previous periods.
- **`abnormal-cli aggregations attack-strategy-breakdown-retrieve`** - Retrieve the breakdown of attacks based on their strategy.
- **`abnormal-cli aggregations attack-vector-breakdown-retrieve`** - Retrieve the breakdown of attacks based on their vectors.
- **`abnormal-cli aggregations attacker-origin-retrieve`** - Retrieve the origin countries of attackers for a given period.
- **`abnormal-cli aggregations dashboard-summary-retrieve`** - Retrieve an aggregated summary of multiple security data points for the dashboard.
- **`abnormal-cli aggregations most-impersonated-employee-non-vip-retrieve`** - Retrieve the most impersonated non-VIP employees for a specified period.
- **`abnormal-cli aggregations most-impersonated-employee-retrieve`** - Retrieve the most impersonated employees for a specified period.
- **`abnormal-cli aggregations most-impersonated-employee-vip-retrieve`** - Retrieve the most impersonated VIP employees for a specified period.
- **`abnormal-cli aggregations most-impersonated-vendor-retrieve`** - Retrieve a list of the most impersonated vendors in attacks.
- **`abnormal-cli aggregations recipient-employees-non-vip-retrieve`** - Retrieve a list of the non-VIP employees who were recipients of attacks, based on their job titles.
- **`abnormal-cli aggregations recipient-employees-retrieve`** - Retrieve a list of the employees who were recipients of attacks, based on their job titles.
- **`abnormal-cli aggregations recipient-employees-vip-retrieve`** - Retrieve a list of the VIP employees who were recipients of attacks, based on their job titles.
- **`abnormal-cli aggregations sender-impersonation-breakdown-retrieve`** - Retrieve a breakdown of attacks based on sender impersonation.
- **`abnormal-cli aggregations trending-attacks-retrieve`** - Retrieve the list of trending attacks for a specified period.

### api_resources

Manage api resources

- **`abnormal-cli api-resources resources-actions-create`** - Execute a specific action on a resource (refresh or validate). Returns 202 Accepted with action ID for tracking.
- **`abnormal-cli api-resources resources-create-create`** - Create a new resource with the specified name and optional description. Returns 201 Created with resource ID.
- **`abnormal-cli api-resources resources-retrieve`** - Retrieve a paginated list of resources with optional filtering using pageSize and pageNumber query parameters.
- **`abnormal-cli api-resources resources-retrieve-2`** - Retrieve detailed information about a specific resource by its UUID.
- **`abnormal-cli api-resources resources-update-partial-update`** - Partially update an existing resource's fields (PATCH). Provide only the fields that need updating.

### auditlogs

Manage auditlogs

- **`abnormal-cli auditlogs`** - Gets a list of Audit Logs for Portal

### cases

APIs to manage Abnormal Cases

- **`abnormal-cli cases create`** - Account Takeover license is required to call this endpoint. Use this to update the status of an abnormal case. The action field is contains the new case status.
- **`abnormal-cli cases retrieve`** - Get a list of Abnormal cases identified by Abnormal Security
- **`abnormal-cli cases retrieve-2`** - Account Takeover license is required to call this endpoint.

### detection360

Manage detection360

- **`abnormal-cli detection360 reports-create`** - Use this to report a detection misclassification judgement by Abnormal Security.  We use this data to improve our models, and also give customers transparency into the frequency of misclassifications.
- **`abnormal-cli detection360 reports-retrieve`** - Get a list of Detection 360 reports that you have submitted and view corresponding details for each case, including report summaries, statuses, message analyses, and more.

### email_search

Manage email search

- **`abnormal-cli email-search search-activities-retrieve`** - List activity logs for search and remediation operations. Optionally filter by tenant_ids query parameter (e.g., ?tenant_ids=123&tenant_ids=456). If tenant_ids is not provided, all authorized tenants are included. The tenant_ids must be a subset of the tenants authorized by the bearer token.
- **`abnormal-cli email-search search-activities-status-retrieve`** - Get detailed status of a specific activity including remediation results. Authorization is automatically determined by the bearer token - if the activity belongs to any tenant authorized by your token, you will be able to access it. The activity_log_id is returned in the response from the remediation endpoint.
- **`abnormal-cli email-search search-create`** - Search for email messages across Abnormal and Quarantine sources. Optionally filter by tenant_ids in the request body. If tenant_ids is not provided, all authorized tenants are searched. The tenant_ids must be a subset of the tenants authorized by the bearer token.

**Key Filter Fields:**
- `body_link`: Filter by URLs found in the email body (e.g., phishing links, suspicious domains)
- `judgement`: Filter by threat classification. Values: 'attack' (confirmed threats), 'borderline' (suspicious but not confirmed), 'spam' (unwanted bulk email), 'graymail' (legitimate bulk email), 'safe' (benign messages)
- `judgement_source`: Filter by detection source. Values: 'ABNORMAL_SYSTEM' (flagged by Abnormal's own detection), 'CUSTOMER_AI_MODEL' (flagged by a customer-defined Custom AI Model). Only supported for `source=abnormal` (not quarantine).
- **`abnormal-cli email-search search-messages-attachments-download-retrieve`** - Download an email attachment for a given message.
- **`abnormal-cli email-search search-messages-eml-retrieve`** - Download the EML file for a specific message by cloud_message_id. Returns the EML file content as message/rfc822 format. For quarantine messages, provide both 'quarantineIdentity' and 'recipientMailbox' query parameters.
- **`abnormal-cli email-search search-remediate-create`** - Remediate email messages by deleting, moving, or submitting them for review. Returns an `activity_log_id` that can be polled via the **Get Activity Status** endpoint.

---

## Two modes of operation

**Specific messages** (`remediate_all=false`):
Provide a `messages` list. Each entry must include `tenant_id`, `raw_message_id`, `mailbox_name`, `native_user_id`, `subject`, `sender`, and `received_time`. The response returns an `activity_log_id`; poll **Get Activity Status** to retrieve per-message results.

**Bulk / remediate-all** (`remediate_all=true`):
Provide `search_filters` instead of `messages`. All messages matching the filters are remediated asynchronously. The response returns an `activity_log_id`; poll **Get Activity Status** to track progress.

---

## Actions

| `action` | Description |
|---|---|
| `delete` | Move messages to the provider's deleted items / recoverable items folder |
| `move_to_inbox` | Move messages to a specified folder (requires `target_folder`) |

To attach a Detection 360 case to a remediation, set `submit_d360_case: true` alongside any `action` above.

---

## Valid action / remediation_reason combinations

| `action` | Allowed `remediation_reason` values |
|---|---|
| `delete` | `false_negative`, `unsolicited`, `other`, `groups_remediation` |
| `move_to_inbox` | `quarantine_release`, `other`, `false_negative` |

---

## Validation rules

- `search_filters` is **required** when `remediate_all=true`.
- `messages` is **required** when `remediate_all=false`.
- `target_folder` is **required** when `action=move_to_inbox`.
- `remediation_reason=quarantine_release` is only valid when `source=quarantine`.
- `search_filters.start_time` must be strictly before `search_filters.end_time`.
- `use_sender_regex=true` in `search_filters` requires `sender_email` to be set.
- `use_recipient_regex=true` in `search_filters` requires `recipient_email` to be set.
- `submit_d360_case=true` requires `remediation_reason=false_negative`. Detection 360 only supports missed-attack inquiries today; other reasons are rejected at the API boundary.
- `submit_d360_case=true` with `remediate_all=true` requires `search_filters.subject` to be set.
- `submit_d360_case=true` with `messages` requires `abnormal_message_uuid` on every message.

---

## Tenant filtering

Optionally provide `tenant_ids` to restrict remediation to a subset of tenants. If omitted, all tenants authorized by the bearer token are included. `tenant_ids` must be a subset of the tenants authorized by the bearer token.

### employee

Manage employee

- **`abnormal-cli employee <email_address>`** - Get employee information

### messages

API to manage message details


### roles

API to retrieve roles from RBAC system

- **`abnormal-cli roles`** - Fetch all roles for an account from RBAC system.

This endpoint retrieves a union of both account-specific roles and
global (Abnormal-defined) roles for the authenticated account from
the RBAC service.

### security-settings

API to retrieve security settings including session timeout configuration

- **`abnormal-cli security-settings`** - Fetch security settings for an account.

This endpoint retrieves security settings including session timeout
configuration (inactivity timeout and max session time).

### soar

Manage soar

- **`abnormal-cli soar`** - Fetch all API tokens for the authenticated customer from the Go Token Management Service.

This endpoint retrieves tokens with response format containing:
- token_id: UUID of the token
- name: Token name
- version: Token version (v1 or v2)
- status: Token status (active, expired, revoked)
- created_at: ISO 8601 creation timestamp
- expires_at: ISO 8601 expiration timestamp
- permissions: List of permission strings (scope)

### spm-v2

Manage spm v2

- **`abnormal-cli spm-v2 posture-catalog-retrieve`** - Get posture catalog containing all available abnormal supported postures
- **`abnormal-cli spm-v2 postures-query-create`** - Get a list of all tenant postures
- **`abnormal-cli spm-v2 postures-retrieve`** - Get detailed information about a specific security posture evaluation
- **`abnormal-cli spm-v2 postures-timeline-retrieve`** - Get timeline of events for a specific security posture
- **`abnormal-cli spm-v2 reports-summary-retrieve`** - Get summary report for all postures
- **`abnormal-cli spm-v2 workflow-logs-raw-json-retrieve`** - Get raw JSON for a workflow log

### threats

APIs to manage threats notified in the Abnormal Threat Log

- **`abnormal-cli threats create`** - Use this to remediate or unremediate a threat. If the request is found to be something which can be processed, the server will return a '202 Accept' with an actionId and status URL in the response. This can be used to check the status of the request.
- **`abnormal-cli threats retrieve`** - Get a list of threats
- **`abnormal-cli threats retrieve-2`** - Get details of a threat

### threats-export

Manage threats export

- **`abnormal-cli threats-export`** - Download data from Threat Log in .csv format

### url-rewrite

Manage url rewrite

- **`abnormal-cli url-rewrite`** - Retrieve paginated click and clickthrough events for URL rewrites. Supports filtering by Unix time range, user email, and event type. Returns events where users clicked on rewritten URLs in email messages.

### users

API to retrieve users from RBAC system

- **`abnormal-cli users`** - Retrieves users for an account from the RBAC user management system.

### vendor-cases

Manage vendor cases

- **`abnormal-cli vendor-cases retrieve`** - Get a list of vendor cases.
- **`abnormal-cli vendor-cases retrieve-2`** - Get details of a vendor case

### vendors

API to manage Vendorbase and threats from Vendors

- **`abnormal-cli vendors`** - Get a list of vendors your organization has interacted with


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
abnormal-cli abuse-mailbox

# JSON for scripting and agents
abnormal-cli abuse-mailbox --json

# Filter to specific fields
abnormal-cli abuse-mailbox --json --select id,name,status

# Dry run  -  show the request without sending
abnormal-cli abuse-mailbox --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
abnormal-cli abuse-mailbox --agent
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
abnormal-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/abnormal-security-client-pp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `ABNORMAL_API_TOKEN` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `abnormal-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `abnormal-cli doctor` to check credentials
- Verify the environment variable is set: `echo $ABNORMAL_API_TOKEN`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call**  -  Token is invalid or expired  -  re-mint it in the portal (Settings → Integrations → Abnormal REST API) and re-export ABNORMAL_API_TOKEN
- **403 Forbidden with a valid token**  -  Your egress IP is not on the integration's IP allowlist  -  add it in the portal's Abnormal REST API integration settings
- **Empty results or 4xx for an EU tenant**  -  EU customers must use the EU host  -  export ABNORMAL_BASE_URL=https://eu.rest.abnormalsecurity.com/v1
- **429 Too Many Requests during sync**  -  Lower the sync window or page count, e.g. sync --resources threats --since 24h --max-pages 5, and retry after the rate-limit window

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**Cortex XSOAR AbnormalSecurity pack**](https://github.com/demisto/content)  -  Python
- [**Elastic Abnormal AI integration**](https://github.com/elastic/integrations)  -  YAML
- [**Splunk SOAR Abnormal Security connector**](https://github.com/splunk-soar-connectors/abnormalsecurity)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
