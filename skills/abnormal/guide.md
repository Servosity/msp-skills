# Abnormal Security CLI

**The full Abnormal Security REST API as an agent-native CLI  -  with a local threat store, ranked SOC triage, and one-shot reporting no SOAR pack offers.**

Every threat, case, vendor, employee, and dashboard operation from Abnormal's official API, plus a synced SQLite store that powers a ranked `triage` queue, joined `employee-risk` and `vendor-risk` investigation views, blocking `remediate-watch` confirmation, and a consolidated `report-snapshot` for client reporting. The incumbent integrations live inside SOAR and SIEM platforms; this runs in your terminal and your agents.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `abnormal-cli` and `abnormal-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/abnormal/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/abnormal/install.ps1 | iex
   ```
3. Verify: `abnormal-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=abnormal). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/abnormal --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/abnormal --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `abnormal-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the abnormal skill from https://github.com/Servosity/msp-skills/tree/main/skills/abnormal. The skill defines how its required CLI (`abnormal-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=abnormal).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `ABNORMAL_API_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/abnormal/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/abnormal/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

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

### Remote MCP over HTTP

`abnormal-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `ABNORMAL_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `ABNORMAL_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export ABNORMAL_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
abnormal-mcp --transport http
```

Set `ABNORMAL_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

## Authentication

Mint a REST API token in the Abnormal portal under Settings → Integrations → Abnormal REST API, then export it as `ABNORMAL_API_TOKEN`. Abnormal enforces IP allowlisting on the integration: add your egress IP in the same portal screen or every call returns 403. EU tenants must point the CLI at the EU host: `export ABNORMAL_BASE_URL=https://eu.rest.abnormalsecurity.com/v1`. You can also store the token by piping it to `abnormal-cli auth set-token` (it reads stdin, never an argument) and check it with the auth status command.

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

### SOC triage that compounds
- **`triage`**  -  Surface the newest, highest-severity, still-unremediated threats first so an analyst starts the shift on what actually matters.

  _Reach for this when asked what email threats need attention now  -  it returns a ranked queue instead of a flat list._

  ```bash
  abnormal-cli triage --since 24h --top 20 --agent
  ```
- **`remediate-watch`**  -  Remediate a threat or case and block until Abnormal reports the action reached a terminal state, with a typed exit code and receipt.

  _Use this when a remediation must be confirmed done, not just submitted  -  exit code 0 means the action completed._

  ```bash
  abnormal-cli remediate-watch threat 184712ab-6d8b-47b3-89d7-a314efef23ff --timeout 5m
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
  abnormal-cli vendor-risk supplier.example.com --agent
  ```

## Recipes

### Morning triage

```bash
abnormal-cli triage --since 24h --top 20 --agent --select items.threatId,items.attackType,items.score
```

Ranked unremediated threats from the local store, narrowed to the three fields an agent needs to decide what to open first.

### Remediate and confirm

```bash
abnormal-cli remediate-watch threat 184712ab-6d8b-47b3-89d7-a314efef23ff --timeout 5m
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
abnormal-cli vendor-risk supplier.example.com --agent
```

Vendor details, recent activity, and open vendor cases joined into one investigation view.

## Usage

Run `abnormal-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `ABNORMAL_CONFIG_DIR`, `ABNORMAL_DATA_DIR`, `ABNORMAL_STATE_DIR`, or `ABNORMAL_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `ABNORMAL_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export ABNORMAL_HOME=/srv/abnormal
abnormal-cli doctor
```

On Windows (PowerShell):

```powershell
$env:ABNORMAL_HOME = "C:\ProgramData\abnormal"
abnormal-cli doctor
```

Under `ABNORMAL_HOME=/srv/abnormal`, the four dirs resolve to `/srv/abnormal/config`, `/srv/abnormal/data`, `/srv/abnormal/state`, and `/srv/abnormal/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:ABNORMAL_HOME = "C:\srv\abnormal"
abnormal-cli doctor
```

Under `$env:ABNORMAL_HOME = "C:\srv\abnormal"`, the four dirs resolve to `C:\srv\abnormal\config`, `C:\srv\abnormal\data`, `C:\srv\abnormal\state`, and `C:\srv\abnormal\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "abnormal": {
      "command": "abnormal-mcp",
      "env": {
        "ABNORMAL_HOME": "/srv/abnormal"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `ABNORMAL_DATA_DIR` overrides an explicit `--home` for that kind. Use `ABNORMAL_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `ABNORMAL_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `abnormal-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are never cached on disk. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `abnormal-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `abnormal-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `ABNORMAL_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `ABNORMAL_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

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


### employee-risk


- **`abnormal-cli employee-risk`** - One account-takeover risk picture per employee: profile, Genome identity analysis, 30-day login pattern, and open cases naming them.


### remediate-watch


- **`abnormal-cli remediate-watch`** - Remediate a threat or case and block until Abnormal reports the action reached a terminal state, with a typed exit code and receipt.


### report-snapshot


- **`abnormal-cli report-snapshot`** - Pull the key dashboard aggregations  -  attacks seen, attacks stopped, impersonation breakdowns, trending attacks  -  into one client-ready table or CSV.


### smoke


- **`abnormal-cli smoke`** - Verify token, base URL (US/EU), and IP allowlist reachability using Abnormal's vendor-supplied test payloads  -  without touching real tenant data.


### triage


- **`abnormal-cli triage`** - Surface the newest, highest-severity, still-unremediated threats first so an analyst starts the shift on what actually matters.


### vendor-risk


- **`abnormal-cli vendor-risk`** - One vendor-email-compromise picture per vendor: details, recent activity, and open vendor cases.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`abnormal-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`abnormal-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`abnormal-cli learnings list`** - Inspect taught rows
- **`abnormal-cli learnings forget <query>`** - Undo a teach
- **`abnormal-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`abnormal-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`abnormal-cli teach-pattern`** - Install a query/resource template up front
- **`abnormal-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `ABNORMAL_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `abnormal-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
abnormal-cli abuse-mailbox

# JSON for scripting and agents
abnormal-cli abuse-mailbox --json
# Filter to specific fields
abnormal-cli abuse-mailbox --json --select abx_message_id,not_analyzed_reason,recipient

# Dry run  -  show the request without sending
abnormal-cli abuse-mailbox --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
abnormal-cli abuse-mailbox --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries when a no-op success is acceptable
- **Explicit confirmation** - `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

## Health Check

```bash
abnormal-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `abnormal-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/abnormal-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\abnormal-cli\config.toml` on Windows; `--home`, `ABNORMAL_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `ABNORMAL_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `ABNORMAL_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `ABNORMAL_API_TOKEN` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `abnormal-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Credential safety

- The `Authorization: Bearer` token and every API request go only to the scheme, host and port of the configured base URL (`https://api.abnormalplatform.com/v1`, or `ABNORMAL_BASE_URL` for the EU host). A redirect to any other host is refused, so the token never reaches it.
- Nothing the CLI or the MCP server prints, writes to a file or to the local store, or sends to a host other than the configured one can carry `ABNORMAL_API_TOKEN`, the stored or config-file token, or `ABNORMAL_MCP_HTTP_TOKEN`, in any encoding. The shared credential kit withholds the whole message, file write, store row or request, and prints "[credkit: withheld from <where>: it carried a credential this program holds]" in its place. Responses are never cached on disk.
- A `--stdin` request body is held as a credential too, whole and line by line. A `--dry-run` preview of it is withheld, and so is an answer that repeats part of it (that write still happens). `feedback --stdin` is refused for the same reason: pass the note as an argument.
- Credentials are stored only in `credentials.toml` and `config.toml`, both written mode 0600; `auth set-token` reads the token from stdin, never from an argument.
- `--dry-run` sends nothing. Over MCP, a tool that changes something in Abnormal (`abnormal_execute`, `remediate_watch`, `import`) returns a preview and sends only with `confirm: true`, and a tool argument can never name a file to read or write.

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
