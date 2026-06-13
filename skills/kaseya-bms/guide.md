# Kaseya BMS CLI

**The first dedicated CLI and MCP server for Kaseya BMS - the full PSA surface plus offline sync, full-text search, and the queue, contract-burn, and unbilled-revenue analytics the web grid can't compute.**

Kaseya BMS has a 433-operation official API and zero CLI ecosystem. This binary covers the whole surface - tickets, CRM, contracts, finance, projects - and mirrors core entities into local SQLite so dispatch questions like 'queue-health', 'stale-tickets', and 'contract-burn' answer instantly without burning the 1500/hour/endpoint rate limit.

## Install

The recommended path installs both the `kaseya-bms-cli` binary and the `pp-kaseya-bms` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install kaseya-bms
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install kaseya-bms --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install kaseya-bms --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install kaseya-bms --agent claude-code
npx -y @mvanhorn/printing-press-library install kaseya-bms --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/project-management/kaseya-bms/cmd/kaseya-bms-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/kaseya-bms-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install kaseya-bms --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-kaseya-bms --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-kaseya-bms --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install kaseya-bms --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/kaseya-bms-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `KASEYA_BMS_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/project-management/kaseya-bms/cmd/kaseya-bms-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "kaseya-bms": {
      "command": "kaseya-bms-mcp",
      "env": {
        "KASEYA_BMS_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

BMS uses short-lived JWTs. Set KASEYA_BMS_USERNAME, KASEYA_BMS_PASSWORD, and KASEYA_BMS_TENANT (your company name from My Settings in BMS), then run `kaseya-bms-cli auth login` to exchange them for a token - it is stored locally and sent as a Bearer header on every call. If you already have a JWT, set KASEYA_BMS_TOKEN directly. Regional tenants set KASEYA_BMS_BASE_URL (https://api.bms.kaseya.com is the default; EMEA uses https://api.bmsemea.kaseya.com, APAC https://api.bmsapac.kaseya.com, legacy Vorex https://api.vorexlogin.kaseya.com). API users with MFA pass --mfa-code at login.

## Quick Start

```bash
# Verify the binary, config, and gateway URL before touching credentials
kaseya-bms-cli doctor --dry-run

# Exchange KASEYA_BMS_USERNAME/PASSWORD/TENANT for a JWT and store it
kaseya-bms-cli auth login

# Mirror recent tickets, accounts, contacts, and contracts into local SQLite
kaseya-bms-cli sync --since 7d

# The dispatcher's morning board: open volume by queue, priority, and status
kaseya-bms-cli queue-health --agent

# Surface tickets nobody has touched in a week before clients notice
kaseya-bms-cli stale-tickets --days 7 --agent

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Service-desk pulse
- **`queue-health`**  -  See open ticket volume by queue, priority, and status in one shot, with stale counts flagged before the morning standup.

  _Reach for this when asked how the service desk looks right now - it answers in one local query instead of paging the tickets endpoint._

  ```bash
  kaseya-bms-cli queue-health --agent
  ```
- **`stale-tickets`**  -  List open tickets that have not been touched in N days, oldest first, with account and assignee so nothing rots in the queue.

  _Use this for SLA-risk and aging-ticket questions instead of listing all tickets and filtering by hand._

  ```bash
  kaseya-bms-cli stale-tickets --days 7 --agent
  ```
- **`workload`**  -  Open and in-progress ticket load per assignee, flagging who is overloaded and who has slack before you dispatch the next ticket.

  _Pick this when deciding who should take a ticket; it is one command instead of one query per technician._

  ```bash
  kaseya-bms-cli workload --agent
  ```

### Money on the table
- **`contract-burn`**  -  Per-contract burn picture: hours consumed, open tickets, and how much of the contract period has elapsed - at-risk agreements surface first.

  _Use this before authorizing work on an account - it answers whether the contract still has hours left, fleet-wide, in one call._

  ```bash
  kaseya-bms-cli contract-burn --agent --select items.account,items.contract,items.hours_consumed
  ```
- **`unbilled`**  -  Billable, approved, not-yet-billed time grouped by account, in hours - the month-end ready-to-bill review without the Excel export.

  _Reach for this for any what-is-ready-to-bill question; the answer is local, grouped, and JSON-shaped._

  ```bash
  kaseya-bms-cli unbilled --agent
  ```
- **`pipeline`**  -  Open opportunities grouped by stage with counts, total and weighted value, and slipped-close flags for the Monday sales call.

  _Use this for pipeline and forecast questions instead of paging the opportunities endpoint and aggregating by hand._

  ```bash
  kaseya-bms-cli pipeline --agent
  ```

## Recipes


### Morning queue triage

```bash
kaseya-bms-cli queue-health --agent
```

One local query answers what the dispatcher's three browser tabs used to: open volume by queue, priority, and status with stale flags.

### Find tickets about to rot

```bash
kaseya-bms-cli stale-tickets --days 7 --agent
```

Oldest-first list of untouched open tickets with account and assignee, straight from the local mirror.

### Contract hours at risk, narrowed for agents

```bash
kaseya-bms-cli contract-burn --agent --select items.account,items.contract,items.hours_consumed
```

Fleet-wide consumed hours and period-elapsed per contract, with --select trimming the payload to the three fields an agent actually needs.

### What is ready to bill

```bash
kaseya-bms-cli unbilled --agent
```

Approved, billable, unbilled hours grouped by account - the month-end Excel ritual as one command.

### Monday pipeline prep

```bash
kaseya-bms-cli pipeline --agent
```

Opportunities by stage with weighted value and slipped-close flags, computed over the synced mirror.

## Usage

Run `kaseya-bms-cli --help` for the full command reference and flag list.

## Commands

### admin

Manage admin

- **`kaseya-bms-cli admin clone-workflow`** - Clone workflow
- **`kaseya-bms-cli admin delete-k1-access-control-mapping`** - Delete k1 access control mapping
- **`kaseya-bms-cli admin delete-service`** - Delete service
- **`kaseya-bms-cli admin delete-services`** - Delete services
- **`kaseya-bms-cli admin delete-teams-channel`** - Delete teams channel
- **`kaseya-bms-cli admin delete-teams-channels`** - Delete teams channels
- **`kaseya-bms-cli admin delete-webhook-configuration`** - Delete webhook configuration
- **`kaseya-bms-cli admin delete-webhooks`** - Delete webhooks
- **`kaseya-bms-cli admin delete-workflow`** - Delete workflow
- **`kaseya-bms-cli admin get-account-codes-lookup`** - Get account codes lookup
- **`kaseya-bms-cli admin get-agent-procedure-logs`** - Get agent procedure logs
- **`kaseya-bms-cli admin get-chart-of-account-types`** - Get chart of account types
- **`kaseya-bms-cli admin get-chart-of-account-types-look-up`** - Get chart of account types look up
- **`kaseya-bms-cli admin get-chart-of-accounts`** - Get chart of accounts
- **`kaseya-bms-cli admin get-chart-of-accounts-lookup`** - Get chart of accounts lookup
- **`kaseya-bms-cli admin get-company-settings`** - Get company settings
- **`kaseya-bms-cli admin get-copilot-configurations`** - Get copilot configurations
- **`kaseya-bms-cli admin get-custom-field-operators`** - Get custom field operators
- **`kaseya-bms-cli admin get-departments-lookup`** - Get departments lookup
- **`kaseya-bms-cli admin get-job-titles-lookup`** - Get job titles lookup
- **`kaseya-bms-cli admin get-k1-access-control-access-groups`** - Get k1 access control access groups
- **`kaseya-bms-cli admin get-k1-access-control-mappings`** - Get k1 access control mappings
- **`kaseya-bms-cli admin get-k1-access-control-settings`** - Get k1 access control settings
- **`kaseya-bms-cli admin get-k1-launcher-url`** - Get k1 launcher url
- **`kaseya-bms-cli admin get-k1-product-organization-mappings`** - Get k1 product organization mappings
- **`kaseya-bms-cli admin get-k1-ssomodules`** - Get k1 ssomodules
- **`kaseya-bms-cli admin get-k1-status`** - Get k1 status
- **`kaseya-bms-cli admin get-related-services-list`** - Get related services list
- **`kaseya-bms-cli admin get-satisfaction-score-options-lookup`** - Get satisfaction score options lookup
- **`kaseya-bms-cli admin get-security-roles-lookup`** - Get security roles lookup
- **`kaseya-bms-cli admin get-service`** - Get service
- **`kaseya-bms-cli admin get-service-categories-lookup`** - Get service categories lookup
- **`kaseya-bms-cli admin get-service-in-progress-job`** - Get service in progress job
- **`kaseya-bms-cli admin get-service-sub-categories-lookup`** - Get service sub categories lookup
- **`kaseya-bms-cli admin get-service-type-options-lookup`** - Get service type options lookup
- **`kaseya-bms-cli admin get-services-list`** - Get services list
- **`kaseya-bms-cli admin get-services-lookup`** - Get services lookup
- **`kaseya-bms-cli admin get-teams-channel`** - Get teams channel
- **`kaseya-bms-cli admin get-teams-channel-lookup`** - Get teams channel lookup
- **`kaseya-bms-cli admin get-teams-channel-status`** - Get teams channel status
- **`kaseya-bms-cli admin get-teams-channels-activity-logs-list`** - Get teams channels activity logs list
- **`kaseya-bms-cli admin get-teams-channels-list`** - Get teams channels list
- **`kaseya-bms-cli admin get-webhook-configuration`** - Get webhook configuration
- **`kaseya-bms-cli admin get-webhook-configurations-list`** - Get webhook configurations list
- **`kaseya-bms-cli admin get-webhook-delivery-log`** - Get webhook delivery log
- **`kaseya-bms-cli admin get-webhook-delivery-logs-list`** - Get webhook delivery logs list
- **`kaseya-bms-cli admin get-webhook-events-lookup`** - Get webhook events lookup
- **`kaseya-bms-cli admin get-workflow`** - Get workflow
- **`kaseya-bms-cli admin get-workflow-time-zone-and-working-hours`** - Get workflow time zone and working hours
- **`kaseya-bms-cli admin get-workflows-list`** - Get workflows list
- **`kaseya-bms-cli admin get-workforce-planner-logs-list`** - Get workforce planner logs list
- **`kaseya-bms-cli admin patch-services`** - Patch services
- **`kaseya-bms-cli admin patch-teams-channel`** - Patch teams channel
- **`kaseya-bms-cli admin patch-webhooks`** - Patch webhooks
- **`kaseya-bms-cli admin patch-workflow`** - Patch workflow
- **`kaseya-bms-cli admin post-copilot-configuration`** - Post copilot configuration
- **`kaseya-bms-cli admin post-k1-access-control-mapping`** - Post k1 access control mapping
- **`kaseya-bms-cli admin post-k1-access-control-settings`** - Post k1 access control settings
- **`kaseya-bms-cli admin post-k1-product-organization-mappings`** - Post k1 product organization mappings
- **`kaseya-bms-cli admin post-service`** - Post service
- **`kaseya-bms-cli admin post-teams-channel`** - Post teams channel
- **`kaseya-bms-cli admin post-webhook-configuration`** - Post webhook configuration
- **`kaseya-bms-cli admin post-webhook-delivery-logs-redelivery`** - Post webhook delivery logs redelivery
- **`kaseya-bms-cli admin post-workflow`** - Post workflow
- **`kaseya-bms-cli admin put-copilot-configuration`** - Put copilot configuration
- **`kaseya-bms-cli admin put-k1-access-control-mapping`** - Put k1 access control mapping
- **`kaseya-bms-cli admin put-k1-access-control-settings`** - Put k1 access control settings
- **`kaseya-bms-cli admin put-service`** - Put service
- **`kaseya-bms-cli admin put-teams-channel`** - Put teams channel
- **`kaseya-bms-cli admin put-webhook-configuration`** - Put webhook configuration
- **`kaseya-bms-cli admin put-workflow`** - Put workflow
- **`kaseya-bms-cli admin search-k1-product-organization-mappings`** - Search k1 product organization mappings
- **`kaseya-bms-cli admin update`** - Update
- **`kaseya-bms-cli admin update-related-services-list`** - Update related services list

### calendar

Manage calendar

- **`kaseya-bms-cli calendar dismiss-workforce-planner-period-warning`** - Dismiss workforce planner period warning
- **`kaseya-bms-cli calendar get-workforce-planner`** - Get workforce planner
- **`kaseya-bms-cli calendar put-workforce-planner-period`** - Put workforce planner period
- **`kaseya-bms-cli calendar split-workforce-planner-period`** - Split workforce planner period

### clientportal

Manage clientportal

- **`kaseya-bms-cli clientportal get-client-portal-ticket-activities`** - Get client portal ticket activities
- **`kaseya-bms-cli clientportal get-client-portal-ticket-by-id`** - Get client portal ticket by id
- **`kaseya-bms-cli clientportal post-client-portal-ticket`** - Post client portal ticket
- **`kaseya-bms-cli clientportal post-client-portal-ticket-note`** - Post client portal ticket note
- **`kaseya-bms-cli clientportal search-client-portal-my-tickets`** - Search client portal my tickets
- **`kaseya-bms-cli clientportal search-client-portal-tickets`** - Search client portal tickets

### crm

Manage crm

- **`kaseya-bms-cli crm delete-contact`** - Delete contact
- **`kaseya-bms-cli crm get-account-alert`** - Get account alert
- **`kaseya-bms-cli crm get-account-contact-summary-info`** - Get account contact summary info
- **`kaseya-bms-cli crm get-account-contacts`** - Get account contacts
- **`kaseya-bms-cli crm get-account-location`** - Get account location
- **`kaseya-bms-cli crm get-account-locations`** - Get account locations
- **`kaseya-bms-cli crm get-account-summary`** - Get account summary
- **`kaseya-bms-cli crm get-accounts`** - Get accounts
- **`kaseya-bms-cli crm get-accounts-list-summary`** - Get accounts list summary
- **`kaseya-bms-cli crm get-activities-due`** - Get activities due
- **`kaseya-bms-cli crm get-all-opportunities`** - Get all opportunities
- **`kaseya-bms-cli crm get-contact-summary`** - Get contact summary
- **`kaseya-bms-cli crm get-contacts-list-summary`** - Get contacts list summary
- **`kaseya-bms-cli crm get-contacts-search`** - Get contacts search
- **`kaseya-bms-cli crm get-dashboard-quotations-summary`** - Get dashboard quotations summary
- **`kaseya-bms-cli crm get-line-item`** - Get line item
- **`kaseya-bms-cli crm get-next-opportunities`** - Get next opportunities
- **`kaseya-bms-cli crm get-open-pipelines`** - Get open pipelines
- **`kaseya-bms-cli crm get-opportunity`** - Get opportunity
- **`kaseya-bms-cli crm get-opportunity-notes`** - Get opportunity notes
- **`kaseya-bms-cli crm get-opportunity-status-lookups`** - Get opportunity status lookups
- **`kaseya-bms-cli crm get-pipeline-totals`** - Get pipeline totals
- **`kaseya-bms-cli crm get-product-quotation`** - Get product quotation
- **`kaseya-bms-cli crm get-product-quotations-list-summary`** - Get product quotations list summary
- **`kaseya-bms-cli crm get-quotation`** - Get quotation
- **`kaseya-bms-cli crm get-quotations-list-summary`** - Get quotations list summary
- **`kaseya-bms-cli crm get-recurring-services-performance`** - Get recurring services performance
- **`kaseya-bms-cli crm get-recurring-services-performance-totals`** - Get recurring services performance totals
- **`kaseya-bms-cli crm get-sales-accelerator-data`** - Get sales accelerator data
- **`kaseya-bms-cli crm get-sales-accelerator-summary`** - Get sales accelerator summary
- **`kaseya-bms-cli crm get-sales-leaderboard`** - Get sales leaderboard
- **`kaseya-bms-cli crm get-top-opportunities`** - Get top opportunities
- **`kaseya-bms-cli crm patch-account`** - Patch account
- **`kaseya-bms-cli crm patch-contact`** - Patch contact
- **`kaseya-bms-cli crm post-account`** - Post account
- **`kaseya-bms-cli crm post-contact-summary-info`** - Post contact summary info
- **`kaseya-bms-cli crm post-opportunity`** - Post opportunity
- **`kaseya-bms-cli crm post-opportunity-note`** - Post opportunity note
- **`kaseya-bms-cli crm post-quotation-line-item`** - Post quotation line item
- **`kaseya-bms-cli crm put-account`** - Put account
- **`kaseya-bms-cli crm put-contact`** - Put contact
- **`kaseya-bms-cli crm put-opportunity`** - Put opportunity
- **`kaseya-bms-cli crm put-opportunity-note`** - Put opportunity note
- **`kaseya-bms-cli crm put-quotation-line-item`** - Put quotation line item

### finance

Manage finance

- **`kaseya-bms-cli finance activate-contract`** - Activate contract
- **`kaseya-bms-cli finance deactivate-contract`** - Deactivate contract
- **`kaseya-bms-cli finance get-contract-lookups`** - Get contract lookups
- **`kaseya-bms-cli finance get-contracts-summary`** - Get contracts summary
- **`kaseya-bms-cli finance get-invoice`** - Get invoice
- **`kaseya-bms-cli finance get-invoice-details-list`** - Get invoice details list
- **`kaseya-bms-cli finance get-invoice-discounts-list`** - Get invoice discounts list
- **`kaseya-bms-cli finance get-invoices-summary-list`** - Get invoices summary list
- **`kaseya-bms-cli finance mark-invoices-as-sent`** - Mark invoices as sent
- **`kaseya-bms-cli finance post-recurring-service-contract`** - Post recurring service contract
- **`kaseya-bms-cli finance post-recurring-services`** - Post recurring services
- **`kaseya-bms-cli finance put-recurring-service-contract`** - Put recurring service contract

### hr

Manage hr

- **`kaseya-bms-cli hr get-assignee-lookups`** - Get assignee lookups
- **`kaseya-bms-cli hr get-assignees`** - Get assignees
- **`kaseya-bms-cli hr get-assignees-count`** - Get assignees count
- **`kaseya-bms-cli hr get-employee-role-lookups`** - Get employee role lookups
- **`kaseya-bms-cli hr get-employees-list-search-select`** - Get employees list search select
- **`kaseya-bms-cli hr get-employees-lookup`** - Get employees lookup

### integration

Manage integration

- **`kaseya-bms-cli integration get-account-codes`** - Get account codes
- **`kaseya-bms-cli integration get-account-codes-by-ids`** - Get account codes by ids
- **`kaseya-bms-cli integration get-bill-details`** - Get bill details
- **`kaseya-bms-cli integration get-bills`** - Get bills
- **`kaseya-bms-cli integration get-class-lists`** - Get class lists
- **`kaseya-bms-cli integration get-class-lists-by-ids`** - Get class lists by ids
- **`kaseya-bms-cli integration get-client-by-ids`** - Get client by ids
- **`kaseya-bms-cli integration get-clients`** - Get clients
- **`kaseya-bms-cli integration get-discounts`** - Get discounts
- **`kaseya-bms-cli integration get-discounts-by-ids`** - Get discounts by ids
- **`kaseya-bms-cli integration get-expenses`** - Get expenses
- **`kaseya-bms-cli integration get-expenses-by-ids`** - Get expenses by ids
- **`kaseya-bms-cli integration get-invoice-details`** - Get invoice details
- **`kaseya-bms-cli integration get-invoices`** - Get invoices
- **`kaseya-bms-cli integration get-job-info`** - Get job info
- **`kaseya-bms-cli integration get-products`** - Get products
- **`kaseya-bms-cli integration get-products-by-ids`** - Get products by ids
- **`kaseya-bms-cli integration get-reimbursement-details`** - Get reimbursement details
- **`kaseya-bms-cli integration get-reimbursements`** - Get reimbursements
- **`kaseya-bms-cli integration get-sales-tax-items`** - Get sales tax items
- **`kaseya-bms-cli integration get-sales-tax-items-by-ids`** - Get sales tax items by ids
- **`kaseya-bms-cli integration get-services`** - Get services
- **`kaseya-bms-cli integration get-services-by-ids`** - Get services by ids
- **`kaseya-bms-cli integration get-vendors`** - Get vendors
- **`kaseya-bms-cli integration get-vendors-by-ids`** - Get vendors by ids
- **`kaseya-bms-cli integration get-work-types`** - Get work types
- **`kaseya-bms-cli integration get-work-types-by-ids`** - Get work types by ids
- **`kaseya-bms-cli integration import-account-codes`** - Import account codes
- **`kaseya-bms-cli integration import-class-lists`** - Import class lists
- **`kaseya-bms-cli integration import-clients`** - Import clients
- **`kaseya-bms-cli integration import-discounts`** - Import discounts
- **`kaseya-bms-cli integration import-expenses`** - Import expenses
- **`kaseya-bms-cli integration import-payments`** - Import payments
- **`kaseya-bms-cli integration import-products`** - Import products
- **`kaseya-bms-cli integration import-services`** - Import services
- **`kaseya-bms-cli integration import-vendors`** - Import vendors
- **`kaseya-bms-cli integration import-work-types`** - Import work types
- **`kaseya-bms-cli integration post-entity-mappings`** - Post entity mappings
- **`kaseya-bms-cli integration update-bill-qb-reference`** - Update bill qb reference
- **`kaseya-bms-cli integration update-invoice-qb-reference`** - Update invoice qb reference
- **`kaseya-bms-cli integration update-reimbursement-qb-reference`** - Update reimbursement qb reference

### integrations

Manage integrations

- **`kaseya-bms-cli integrations get-distributors-pricing`** - Get distributors pricing
- **`kaseya-bms-cli integrations get-distributors-status`** - Get distributors status
- **`kaseya-bms-cli integrations get-etilize-product`** - Get etilize product
- **`kaseya-bms-cli integrations get-itg-account-lookup`** - Get itg account lookup
- **`kaseya-bms-cli integrations get-itg-asset-lookup`** - Get itg asset lookup
- **`kaseya-bms-cli integrations get-itg-checklist-tasks`** - Get itg checklist tasks
- **`kaseya-bms-cli integrations get-itg-checklists`** - Get itg checklists
- **`kaseya-bms-cli integrations get-itg-contact-lookup`** - Get itg contact lookup
- **`kaseya-bms-cli integrations get-itg-contact-notes`** - Get itg contact notes
- **`kaseya-bms-cli integrations get-itg-location-lookup`** - Get itg location lookup
- **`kaseya-bms-cli integrations get-itg-organization-notes`** - Get itg organization notes
- **`kaseya-bms-cli integrations get-itg-status`** - Get itg status
- **`kaseya-bms-cli integrations get-itgaccess-info`** - Get itgaccess info
- **`kaseya-bms-cli integrations get-itgpassword-value`** - Get itgpassword value
- **`kaseya-bms-cli integrations get-itgsuggested-resources`** - Get itgsuggested resources
- **`kaseya-bms-cli integrations get-itgsuggested-resources-count`** - Get itgsuggested resources count
- **`kaseya-bms-cli integrations get-survey-settings`** - Get survey settings
- **`kaseya-bms-cli integrations get-ticket-global-search`** - Get ticket global search
- **`kaseya-bms-cli integrations get-ticket-sync`** - Get ticket sync
- **`kaseya-bms-cli integrations get-tickets-sync`** - Get tickets sync
- **`kaseya-bms-cli integrations get-vsa-access-info`** - Get vsa access info
- **`kaseya-bms-cli integrations put-itgcontact-notes`** - Put itgcontact notes
- **`kaseya-bms-cli integrations put-itgorganization-notes`** - Put itgorganization notes
- **`kaseya-bms-cli integrations search-products`** - Search products

### inventory

Manage inventory

- **`kaseya-bms-cli inventory get-polookups`** - Get polookups
- **`kaseya-bms-cli inventory get-pricing-levels`** - Get pricing levels
- **`kaseya-bms-cli inventory get-product-categories-lookup`** - Get product categories lookup
- **`kaseya-bms-cli inventory get-product-default-cost`** - Get product default cost
- **`kaseya-bms-cli inventory get-product-weighted-cost`** - Get product weighted cost
- **`kaseya-bms-cli inventory get-products-in-stock`** - Get products in stock
- **`kaseya-bms-cli inventory get-products-list-search-select`** - Get products list search select
- **`kaseya-bms-cli inventory get-supplier-lookups`** - Get supplier lookups
- **`kaseya-bms-cli inventory get-warehouse-lookups`** - Get warehouse lookups
- **`kaseya-bms-cli inventory post-product-categories`** - Post product categories

### kaseya-one

Manage kaseya one

- **`kaseya-bms-cli kaseya-one get-k1-resource-lookup`** - Get k1 resource lookup
- **`kaseya-bms-cli kaseya-one get-k1-resources-lookup`** - Get k1 resources lookup
- **`kaseya-bms-cli kaseya-one get-k1-role-lookup`** - Get k1 role lookup
- **`kaseya-bms-cli kaseya-one get-k1-roles-lookup`** - Get k1 roles lookup
- **`kaseya-bms-cli kaseya-one get-k1-ticket-statuses-lookup`** - Get k1 ticket statuses lookup
- **`kaseya-bms-cli kaseya-one get-k1-ticket-statuses-lookup-kaseyaone`** - Get k1 ticket statuses lookup kaseyaone
- **`kaseya-bms-cli kaseya-one get-work-type-lookup`** - Get work type lookup
- **`kaseya-bms-cli kaseya-one get-work-types-lookup`** - Get work types lookup
- **`kaseya-bms-cli kaseya-one k1-get-users-list`** - K1 get users list
- **`kaseya-bms-cli kaseya-one post-k1-centralized-configurations`** - Post k1 centralized configurations
- **`kaseya-bms-cli kaseya-one post-k1-gorgon-provisioning-notifications`** - Post k1 gorgon provisioning notifications
- **`kaseya-bms-cli kaseya-one post-k1-ticket-action-notifications`** - Post k1 ticket action notifications
- **`kaseya-bms-cli kaseya-one post-k1-user-deprovisioning-notifications`** - Post k1 user deprovisioning notifications

### listing

Manage listing

- **`kaseya-bms-cli listing get-columns`** - Get columns
- **`kaseya-bms-cli listing get-search-columns`** - Get search columns
- **`kaseya-bms-cli listing get-search-combo-items`** - Get search combo items

### my

Manage my

- **`kaseya-bms-cli my get-expense-sheets-lookups`** - Get expense sheets lookups
- **`kaseya-bms-cli my post-expense-sheet`** - Post expense sheet

### project

Manage project

- **`kaseya-bms-cli project delete-status`** - Delete status
- **`kaseya-bms-cli project get`** - Get
- **`kaseya-bms-cli project get-list-summary`** - Get list summary
- **`kaseya-bms-cli project get-lookup`** - Get lookup
- **`kaseya-bms-cli project get-status`** - Get status
- **`kaseya-bms-cli project get-statuses-list`** - Get statuses list
- **`kaseya-bms-cli project get-statuses-lookup`** - Get statuses lookup
- **`kaseya-bms-cli project get-task-related-items`** - Get task related items
- **`kaseya-bms-cli project get-tasks-list`** - Get tasks list
- **`kaseya-bms-cli project post-status`** - Post status
- **`kaseya-bms-cli project put-status`** - Put status
- **`kaseya-bms-cli project put-status-order`** - Put status order
- **`kaseya-bms-cli project search-and-select-task-list`** - Search and select task list

### rmm

Manage rmm

- **`kaseya-bms-cli rmm create`** - Create
- **`kaseya-bms-cli rmm get`** - Get
- **`kaseya-bms-cli rmm get-rmmsettings`** - Get rmmsettings
- **`kaseya-bms-cli rmm post-alert`** - Post alert
- **`kaseya-bms-cli rmm update`** - Update
- **`kaseya-bms-cli rmm update-integrationtype`** - Update integrationtype

### security

Manage security

- **`kaseya-bms-cli security authenticate`** - Authenticate
- **`kaseya-bms-cli security refresh-token`** - Refresh token
- **`kaseya-bms-cli security sso-status`** - Sso status
- **`kaseya-bms-cli security user-info`** - User info

### servicedesk

Manage servicedesk

- **`kaseya-bms-cli servicedesk add-related-tickets`** - Add related tickets
- **`kaseya-bms-cli servicedesk add-ticket-tasks`** - Add ticket tasks
- **`kaseya-bms-cli servicedesk assign-ticket`** - Assign ticket
- **`kaseya-bms-cli servicedesk create`** - Create
- **`kaseya-bms-cli servicedesk create-tickets`** - Create tickets
- **`kaseya-bms-cli servicedesk create-tickets-2`** - Create tickets 2
- **`kaseya-bms-cli servicedesk create-tickets-3`** - Create tickets 3
- **`kaseya-bms-cli servicedesk create-tickets-4`** - Create tickets 4
- **`kaseya-bms-cli servicedesk delete-hardware-asset`** - Delete hardware asset
- **`kaseya-bms-cli servicedesk delete-related-ticket`** - Delete related ticket
- **`kaseya-bms-cli servicedesk delete-service-call`** - Delete service call
- **`kaseya-bms-cli servicedesk delete-service-calls-and-to-dos`** - Delete service calls and to dos
- **`kaseya-bms-cli servicedesk delete-ticket`** - Delete ticket
- **`kaseya-bms-cli servicedesk delete-ticket-charge`** - Delete ticket charge
- **`kaseya-bms-cli servicedesk delete-ticket-checklist-item`** - Delete ticket checklist item
- **`kaseya-bms-cli servicedesk delete-ticket-checklist-items`** - Delete ticket checklist items
- **`kaseya-bms-cli servicedesk delete-ticket-expense`** - Delete ticket expense
- **`kaseya-bms-cli servicedesk delete-ticket-note`** - Delete ticket note
- **`kaseya-bms-cli servicedesk delete-ticket-tasks`** - Delete ticket tasks
- **`kaseya-bms-cli servicedesk delete-ticket-time-entry`** - Delete ticket time entry
- **`kaseya-bms-cli servicedesk delete-tickets`** - Delete tickets
- **`kaseya-bms-cli servicedesk delete-to-do`** - Delete to do
- **`kaseya-bms-cli servicedesk get`** - Get
- **`kaseya-bms-cli servicedesk get-agent-procedure-audits`** - Get agent procedure audits
- **`kaseya-bms-cli servicedesk get-batch-action-logs`** - Get batch action logs
- **`kaseya-bms-cli servicedesk get-ccs`** - Get ccs
- **`kaseya-bms-cli servicedesk get-device-status`** - Get device status
- **`kaseya-bms-cli servicedesk get-hardware-asset`** - Get hardware asset
- **`kaseya-bms-cli servicedesk get-hardware-asset-tickets`** - Get hardware asset tickets
- **`kaseya-bms-cli servicedesk get-hardware-assets-list-search-select`** - Get hardware assets list search select
- **`kaseya-bms-cli servicedesk get-my-tickets`** - Get my tickets
- **`kaseya-bms-cli servicedesk get-product-charge-delivery-history-by-charge-id`** - Get product charge delivery history by charge id
- **`kaseya-bms-cli servicedesk get-queue-tickets`** - Get queue tickets
- **`kaseya-bms-cli servicedesk get-related-alerts`** - Get related alerts
- **`kaseya-bms-cli servicedesk get-related-tickets`** - Get related tickets
- **`kaseya-bms-cli servicedesk get-related-tickets-count`** - Get related tickets count
- **`kaseya-bms-cli servicedesk get-service-call`** - Get service call
- **`kaseya-bms-cli servicedesk get-service-calls`** - Get service calls
- **`kaseya-bms-cli servicedesk get-software-assets-list-search-select`** - Get software assets list search select
- **`kaseya-bms-cli servicedesk get-ticket`** - Get ticket
- **`kaseya-bms-cli servicedesk get-ticket-activities`** - Get ticket activities
- **`kaseya-bms-cli servicedesk get-ticket-charge`** - Get ticket charge
- **`kaseya-bms-cli servicedesk get-ticket-charge-product-summary`** - Get ticket charge product summary
- **`kaseya-bms-cli servicedesk get-ticket-checklist-items-by-ticket-id`** - Get ticket checklist items by ticket id
- **`kaseya-bms-cli servicedesk get-ticket-expense`** - Get ticket expense
- **`kaseya-bms-cli servicedesk get-ticket-expenses-charges`** - Get ticket expenses charges
- **`kaseya-bms-cli servicedesk get-ticket-logs`** - Get ticket logs
- **`kaseya-bms-cli servicedesk get-ticket-note`** - Get ticket note
- **`kaseya-bms-cli servicedesk get-ticket-note-template-details-by-ticket-id`** - Get ticket note template details by ticket id
- **`kaseya-bms-cli servicedesk get-ticket-note-template-lookups`** - Get ticket note template lookups
- **`kaseya-bms-cli servicedesk get-ticket-notes`** - Get ticket notes
- **`kaseya-bms-cli servicedesk get-ticket-product-charge-assets-by-charge-id`** - Get ticket product charge assets by charge id
- **`kaseya-bms-cli servicedesk get-ticket-service-calls-to-dos`** - Get ticket service calls to dos
- **`kaseya-bms-cli servicedesk get-ticket-slainfo`** - Get ticket slainfo
- **`kaseya-bms-cli servicedesk get-ticket-survey-scores`** - Get ticket survey scores
- **`kaseya-bms-cli servicedesk get-ticket-tabs-indicators`** - Get ticket tabs indicators
- **`kaseya-bms-cli servicedesk get-ticket-tasks`** - Get ticket tasks
- **`kaseya-bms-cli servicedesk get-ticket-tasks-count`** - Get ticket tasks count
- **`kaseya-bms-cli servicedesk get-ticket-template`** - Get ticket template
- **`kaseya-bms-cli servicedesk get-ticket-template-lookups`** - Get ticket template lookups
- **`kaseya-bms-cli servicedesk get-ticket-time-entries`** - Get ticket time entries
- **`kaseya-bms-cli servicedesk get-ticket-time-entry`** - Get ticket time entry
- **`kaseya-bms-cli servicedesk get-ticket-time-entry-template-lookups`** - Get ticket time entry template lookups
- **`kaseya-bms-cli servicedesk get-ticket-timelog-template-details-by-ticket-id`** - Get ticket timelog template details by ticket id
- **`kaseya-bms-cli servicedesk get-tickets`** - Get tickets
- **`kaseya-bms-cli servicedesk get-tickets-2`** - Get tickets 2
- **`kaseya-bms-cli servicedesk get-tickets-count`** - Get tickets count
- **`kaseya-bms-cli servicedesk get-tickets-count-by-assignee-priority`** - Get tickets count by assignee priority
- **`kaseya-bms-cli servicedesk get-tickets-count-by-basic-status`** - Get tickets count by basic status
- **`kaseya-bms-cli servicedesk get-tickets-count-by-issue-type`** - Get tickets count by issue type
- **`kaseya-bms-cli servicedesk get-tickets-count-by-priority`** - Get tickets count by priority
- **`kaseya-bms-cli servicedesk get-tickets-count-by-queue`** - Get tickets count by queue
- **`kaseya-bms-cli servicedesk get-tickets-count-by-status`** - Get tickets count by status
- **`kaseya-bms-cli servicedesk get-tickets-due`** - Get tickets due
- **`kaseya-bms-cli servicedesk get-tickets-list-search-select`** - Get tickets list search select
- **`kaseya-bms-cli servicedesk get-tickets-list-summary`** - Get tickets list summary
- **`kaseya-bms-cli servicedesk get-tickets-upcoming`** - Get tickets upcoming
- **`kaseya-bms-cli servicedesk get-time-logged-by-technician`** - Get time logged by technician
- **`kaseya-bms-cli servicedesk get-to-do`** - Get to do
- **`kaseya-bms-cli servicedesk get-vsa-hardware-asset-tickets`** - Get vsa hardware asset tickets
- **`kaseya-bms-cli servicedesk get-workflow-logs`** - Get workflow logs
- **`kaseya-bms-cli servicedesk merge-tickets`** - Merge tickets
- **`kaseya-bms-cli servicedesk patch-ticket`** - Patch ticket
- **`kaseya-bms-cli servicedesk patch-tickets`** - Patch tickets
- **`kaseya-bms-cli servicedesk post-checklist-tasks`** - Post checklist tasks
- **`kaseya-bms-cli servicedesk post-checklist-with-tasks`** - Post checklist with tasks
- **`kaseya-bms-cli servicedesk post-hardware-asset`** - Post hardware asset
- **`kaseya-bms-cli servicedesk post-po`** - Post po
- **`kaseya-bms-cli servicedesk post-product-deliver`** - Post product deliver
- **`kaseya-bms-cli servicedesk post-service-call`** - Post service call
- **`kaseya-bms-cli servicedesk post-ticket`** - Post ticket
- **`kaseya-bms-cli servicedesk post-ticket-charge`** - Post ticket charge
- **`kaseya-bms-cli servicedesk post-ticket-checklist-item`** - Post ticket checklist item
- **`kaseya-bms-cli servicedesk post-ticket-expense`** - Post ticket expense
- **`kaseya-bms-cli servicedesk post-ticket-note`** - Post ticket note
- **`kaseya-bms-cli servicedesk post-ticket-time-entry`** - Post ticket time entry
- **`kaseya-bms-cli servicedesk post-to-do`** - Post to do
- **`kaseya-bms-cli servicedesk put-hardware-asset`** - Put hardware asset
- **`kaseya-bms-cli servicedesk put-service-call`** - Put service call
- **`kaseya-bms-cli servicedesk put-ticket`** - Put ticket
- **`kaseya-bms-cli servicedesk put-ticket-charge`** - Put ticket charge
- **`kaseya-bms-cli servicedesk put-ticket-checklist-item`** - Put ticket checklist item
- **`kaseya-bms-cli servicedesk put-ticket-expense`** - Put ticket expense
- **`kaseya-bms-cli servicedesk put-ticket-note`** - Put ticket note
- **`kaseya-bms-cli servicedesk put-ticket-time-entry`** - Put ticket time entry
- **`kaseya-bms-cli servicedesk put-to-do`** - Put to do
- **`kaseya-bms-cli servicedesk resolve-ticket`** - Resolve ticket
- **`kaseya-bms-cli servicedesk search-my-tickets`** - Search my tickets
- **`kaseya-bms-cli servicedesk search-queue-tickets`** - Search queue tickets
- **`kaseya-bms-cli servicedesk search-tickets`** - Search tickets
- **`kaseya-bms-cli servicedesk update`** - Update
- **`kaseya-bms-cli servicedesk update-tickets`** - Update tickets

### system

Manage system

- **`kaseya-bms-cli system create-tenant-lookup`** - Create tenant lookup
- **`kaseya-bms-cli system delete-attachment`** - Delete attachment
- **`kaseya-bms-cli system delete-attachments`** - Delete attachments
- **`kaseya-bms-cli system delete-temp-attachments`** - Delete temp attachments
- **`kaseya-bms-cli system delete-view`** - Delete view
- **`kaseya-bms-cli system download-attachment`** - Download attachment
- **`kaseya-bms-cli system download-profile-picture`** - Download profile picture
- **`kaseya-bms-cli system get-account-types-lookup`** - Get account types lookup
- **`kaseya-bms-cli system get-all-batch-action-logs`** - Get all batch action logs
- **`kaseya-bms-cli system get-approval-routes-lookup`** - Get approval routes lookup
- **`kaseya-bms-cli system get-attachment`** - Get attachment
- **`kaseya-bms-cli system get-attachments`** - Get attachments
- **`kaseya-bms-cli system get-colors-lookup`** - Get colors lookup
- **`kaseya-bms-cli system get-custom-fields-data`** - Get custom fields data
- **`kaseya-bms-cli system get-custom-fields-meta-data`** - Get custom fields meta data
- **`kaseya-bms-cli system get-email-template-lookups`** - Get email template lookups
- **`kaseya-bms-cli system get-expense-type-lookups`** - Get expense type lookups
- **`kaseya-bms-cli system get-issue-sub-type-lookups`** - Get issue sub type lookups
- **`kaseya-bms-cli system get-issue-type-lookups`** - Get issue type lookups
- **`kaseya-bms-cli system get-locations-lookup`** - Get locations lookup
- **`kaseya-bms-cli system get-lookups`** - Get lookups
- **`kaseya-bms-cli system get-priority-lookups`** - Get priority lookups
- **`kaseya-bms-cli system get-queue-lookups`** - Get queue lookups
- **`kaseya-bms-cli system get-role-lookups`** - Get role lookups
- **`kaseya-bms-cli system get-scheduler-job-status`** - Get scheduler job status
- **`kaseya-bms-cli system get-settings`** - Get settings
- **`kaseya-bms-cli system get-slalookups`** - Get slalookups
- **`kaseya-bms-cli system get-status-lookups`** - Get status lookups
- **`kaseya-bms-cli system get-tenant-lookups`** - Get tenant lookups
- **`kaseya-bms-cli system get-view-details-values`** - Get view details values
- **`kaseya-bms-cli system get-view-search-values`** - Get view search values
- **`kaseya-bms-cli system get-view-share-settings`** - Get view share settings
- **`kaseya-bms-cli system get-views`** - Get views
- **`kaseya-bms-cli system get-work-type-lookups`** - Get work type lookups
- **`kaseya-bms-cli system post`** - Post
- **`kaseya-bms-cli system post-event-log`** - Post event log
- **`kaseya-bms-cli system post-temp-attachment`** - Post temp attachment
- **`kaseya-bms-cli system post-view`** - Post view
- **`kaseya-bms-cli system put-attachment`** - Put attachment
- **`kaseya-bms-cli system put-custom-field-data`** - Put custom field data
- **`kaseya-bms-cli system put-view`** - Put view
- **`kaseya-bms-cli system put-view-info`** - Put view info
- **`kaseya-bms-cli system put-view-share-settings`** - Put view share settings

### timelogs

Manage timelogs

- **`kaseya-bms-cli timelogs`** - Get all time logs


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
kaseya-bms-cli project get <id>

# JSON for scripting and agents
kaseya-bms-cli project get <id> --json

# Filter to specific fields
kaseya-bms-cli project get <id> --json --select id,name,status

# Dry run  -  show the request without sending
kaseya-bms-cli project get <id> --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
kaseya-bms-cli project get <id> --agent
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
kaseya-bms-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/bms-pp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `KASEYA_BMS_TOKEN` | per_call | No | Set to your API credential. |
| `KASEYA_BMS_BEARER_AUTH` | per_call | No | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `kaseya-bms-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `kaseya-bms-cli doctor` to check credentials
- Verify the environment variable is set: `echo $KASEYA_BMS_TOKEN`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Security Error (code 978001) on every call**  -  Your JWT expired - run `kaseya-bms-cli auth login` again, or refresh KASEYA_BMS_TOKEN
- **400 'Tenant can't be empty' at login**  -  Set KASEYA_BMS_TENANT to your company name exactly as shown in BMS under My Settings
- **Rate-limited after heavy listing**  -  BMS caps V2 calls at 1500/hour/endpoint - run `kaseya-bms-cli sync` once and use queue-health/stale-tickets/search against the local mirror
- **Login succeeds but data is empty or 404s**  -  You may be on the wrong regional gateway - set KASEYA_BMS_BASE_URL to your region (api.bmsemea.kaseya.com, api.bmsapac.kaseya.com, or api.vorexlogin.kaseya.com)
- **Staleness/age numbers look off by a few hours**  -  BMS serializes some timestamps without zone info; the CLI treats them as UTC. If your tenant emits local time, expect age/staleness boundaries to shift by your UTC offset

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**huntresslabs/kaseya-ruby**](https://github.com/huntresslabs/kaseya-ruby)  -  Ruby
- [**Twoshoe/kaseya**](https://github.com/Twoshoe/kaseya)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
