---
name: kaseya-bms
description: "The first dedicated CLI and MCP server for Kaseya BMS - the full PSA surface plus offline sync, full-text search, and the queue, contract-burn, and unbilled-revenue analytics the web grid can't compute. Trigger phrases: `check the BMS queue`, `stale tickets in Kaseya BMS`, `how many contract hours are left`, `unbilled time in BMS`, `BMS sales pipeline`, `use kaseya-bms`, `run kaseya-bms`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Kaseya BMS"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - kaseya-bms-cli
    install:
      - kind: script
        bins: [kaseya-bms-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-bms/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-bms/install.ps1
---

# Kaseya BMS - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `kaseya-bms-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-bms/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-bms/install.ps1 | iex
   ```
3. Verify: `kaseya-bms-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `kaseya-bms-cli` and `kaseya-bms-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Kaseya BMS has a 433-operation official API and zero CLI ecosystem. This binary covers the whole surface - tickets, CRM, contracts, finance, projects - and mirrors core entities into local SQLite so dispatch questions like 'queue-health', 'stale-tickets', and 'contract-burn' answer instantly without burning the 500/hour/endpoint rate limit.

## When to Use This CLI

Use this CLI when an agent or operator needs Kaseya BMS PSA data or actions from the terminal: triaging service-desk queues, checking contract hours before authorizing work, reviewing unbilled time, walking the sales pipeline, or creating and updating tickets. The local SQLite mirror makes it the right choice for repeated analytical questions that would otherwise hammer the rate-limited API.

## Anti-triggers

Do not use this CLI for:
- Kaseya VSA / RMM agent operations (endpoints, patches, scripts) - this CLI only covers the BMS PSA
- Datto Autotask PSA workflows - different product, use an Autotask tool
- IT Glue documentation management - BMS only exposes the integration link, not the IT Glue API
- Sending customer-facing invoices or emails - generate the data here, deliver through BMS itself

## Unique Capabilities

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

## Command Reference

**admin**  -  Manage admin

- `kaseya-bms-cli admin clone-workflow`  -  Clone workflow
- `kaseya-bms-cli admin delete-k1-access-control-mapping`  -  Delete k1 access control mapping
- `kaseya-bms-cli admin delete-service`  -  Delete service
- `kaseya-bms-cli admin delete-services`  -  Delete services
- `kaseya-bms-cli admin delete-teams-channel`  -  Delete teams channel
- `kaseya-bms-cli admin delete-teams-channels`  -  Delete teams channels
- `kaseya-bms-cli admin delete-webhook-configuration`  -  Delete webhook configuration
- `kaseya-bms-cli admin delete-webhooks`  -  Delete webhooks
- `kaseya-bms-cli admin delete-workflow`  -  Delete workflow
- `kaseya-bms-cli admin get-account-codes-lookup`  -  Get account codes lookup
- `kaseya-bms-cli admin get-agent-procedure-logs`  -  Get agent procedure logs
- `kaseya-bms-cli admin get-chart-of-account-types`  -  Get chart of account types
- `kaseya-bms-cli admin get-chart-of-account-types-look-up`  -  Get chart of account types look up
- `kaseya-bms-cli admin get-chart-of-accounts`  -  Get chart of accounts
- `kaseya-bms-cli admin get-chart-of-accounts-lookup`  -  Get chart of accounts lookup
- `kaseya-bms-cli admin get-company-settings`  -  Get company settings
- `kaseya-bms-cli admin get-copilot-configurations`  -  Get copilot configurations
- `kaseya-bms-cli admin get-custom-field-operators`  -  Get custom field operators
- `kaseya-bms-cli admin get-departments-lookup`  -  Get departments lookup
- `kaseya-bms-cli admin get-job-titles-lookup`  -  Get job titles lookup
- `kaseya-bms-cli admin get-k1-access-control-access-groups`  -  Get k1 access control access groups
- `kaseya-bms-cli admin get-k1-access-control-mappings`  -  Get k1 access control mappings
- `kaseya-bms-cli admin get-k1-access-control-settings`  -  Get k1 access control settings
- `kaseya-bms-cli admin get-k1-launcher-url`  -  Get k1 launcher url
- `kaseya-bms-cli admin get-k1-product-organization-mappings`  -  Get k1 product organization mappings
- `kaseya-bms-cli admin get-k1-ssomodules`  -  Get k1 ssomodules
- `kaseya-bms-cli admin get-k1-status`  -  Get k1 status
- `kaseya-bms-cli admin get-related-services-list`  -  Get related services list
- `kaseya-bms-cli admin get-satisfaction-score-options-lookup`  -  Get satisfaction score options lookup
- `kaseya-bms-cli admin get-security-roles-lookup`  -  Get security roles lookup
- `kaseya-bms-cli admin get-service`  -  Get service
- `kaseya-bms-cli admin get-service-categories-lookup`  -  Get service categories lookup
- `kaseya-bms-cli admin get-service-in-progress-job`  -  Get service in progress job
- `kaseya-bms-cli admin get-service-sub-categories-lookup`  -  Get service sub categories lookup
- `kaseya-bms-cli admin get-service-type-options-lookup`  -  Get service type options lookup
- `kaseya-bms-cli admin get-services-list`  -  Get services list
- `kaseya-bms-cli admin get-services-lookup`  -  Get services lookup
- `kaseya-bms-cli admin get-teams-channel`  -  Get teams channel
- `kaseya-bms-cli admin get-teams-channel-lookup`  -  Get teams channel lookup
- `kaseya-bms-cli admin get-teams-channel-status`  -  Get teams channel status
- `kaseya-bms-cli admin get-teams-channels-activity-logs-list`  -  Get teams channels activity logs list
- `kaseya-bms-cli admin get-teams-channels-list`  -  Get teams channels list
- `kaseya-bms-cli admin get-webhook-configuration`  -  Get webhook configuration
- `kaseya-bms-cli admin get-webhook-configurations-list`  -  Get webhook configurations list
- `kaseya-bms-cli admin get-webhook-delivery-log`  -  Get webhook delivery log
- `kaseya-bms-cli admin get-webhook-delivery-logs-list`  -  Get webhook delivery logs list
- `kaseya-bms-cli admin get-webhook-events-lookup`  -  Get webhook events lookup
- `kaseya-bms-cli admin get-workflow`  -  Get workflow
- `kaseya-bms-cli admin get-workflow-time-zone-and-working-hours`  -  Get workflow time zone and working hours
- `kaseya-bms-cli admin get-workflows-list`  -  Get workflows list
- `kaseya-bms-cli admin get-workforce-planner-logs-list`  -  Get workforce planner logs list
- `kaseya-bms-cli admin patch-services`  -  Patch services
- `kaseya-bms-cli admin patch-teams-channel`  -  Patch teams channel
- `kaseya-bms-cli admin patch-webhooks`  -  Patch webhooks
- `kaseya-bms-cli admin patch-workflow`  -  Patch workflow
- `kaseya-bms-cli admin post-copilot-configuration`  -  Post copilot configuration
- `kaseya-bms-cli admin post-k1-access-control-mapping`  -  Post k1 access control mapping
- `kaseya-bms-cli admin post-k1-access-control-settings`  -  Post k1 access control settings
- `kaseya-bms-cli admin post-k1-product-organization-mappings`  -  Post k1 product organization mappings
- `kaseya-bms-cli admin post-service`  -  Post service
- `kaseya-bms-cli admin post-teams-channel`  -  Post teams channel
- `kaseya-bms-cli admin post-webhook-configuration`  -  Post webhook configuration
- `kaseya-bms-cli admin post-webhook-delivery-logs-redelivery`  -  Post webhook delivery logs redelivery
- `kaseya-bms-cli admin post-workflow`  -  Post workflow
- `kaseya-bms-cli admin put-copilot-configuration`  -  Put copilot configuration
- `kaseya-bms-cli admin put-k1-access-control-mapping`  -  Put k1 access control mapping
- `kaseya-bms-cli admin put-k1-access-control-settings`  -  Put k1 access control settings
- `kaseya-bms-cli admin put-service`  -  Put service
- `kaseya-bms-cli admin put-teams-channel`  -  Put teams channel
- `kaseya-bms-cli admin put-webhook-configuration`  -  Put webhook configuration
- `kaseya-bms-cli admin put-workflow`  -  Put workflow
- `kaseya-bms-cli admin search-k1-product-organization-mappings`  -  Search k1 product organization mappings
- `kaseya-bms-cli admin update`  -  Update
- `kaseya-bms-cli admin update-related-services-list`  -  Update related services list

**calendar**  -  Manage calendar

- `kaseya-bms-cli calendar dismiss-workforce-planner-period-warning`  -  Dismiss workforce planner period warning
- `kaseya-bms-cli calendar get-workforce-planner`  -  Get workforce planner
- `kaseya-bms-cli calendar put-workforce-planner-period`  -  Put workforce planner period
- `kaseya-bms-cli calendar split-workforce-planner-period`  -  Split workforce planner period

**clientportal**  -  Manage clientportal

- `kaseya-bms-cli clientportal get-client-portal-ticket-activities`  -  Get client portal ticket activities
- `kaseya-bms-cli clientportal get-client-portal-ticket-by-id`  -  Get client portal ticket by id
- `kaseya-bms-cli clientportal post-client-portal-ticket`  -  Post client portal ticket
- `kaseya-bms-cli clientportal post-client-portal-ticket-note`  -  Post client portal ticket note
- `kaseya-bms-cli clientportal search-client-portal-my-tickets`  -  Search client portal my tickets
- `kaseya-bms-cli clientportal search-client-portal-tickets`  -  Search client portal tickets

**crm**  -  Manage crm

- `kaseya-bms-cli crm delete-contact`  -  Delete contact
- `kaseya-bms-cli crm get-account-alert`  -  Get account alert
- `kaseya-bms-cli crm get-account-contact-summary-info`  -  Get account contact summary info
- `kaseya-bms-cli crm get-account-contacts`  -  Get account contacts
- `kaseya-bms-cli crm get-account-location`  -  Get account location
- `kaseya-bms-cli crm get-account-locations`  -  Get account locations
- `kaseya-bms-cli crm get-account-summary`  -  Get account summary
- `kaseya-bms-cli crm get-accounts`  -  Get accounts
- `kaseya-bms-cli crm get-accounts-list-summary`  -  Get accounts list summary
- `kaseya-bms-cli crm get-activities-due`  -  Get activities due
- `kaseya-bms-cli crm get-all-opportunities`  -  Get all opportunities
- `kaseya-bms-cli crm get-contact-summary`  -  Get contact summary
- `kaseya-bms-cli crm get-contacts-list-summary`  -  Get contacts list summary
- `kaseya-bms-cli crm get-contacts-search`  -  Get contacts search
- `kaseya-bms-cli crm get-dashboard-quotations-summary`  -  Get dashboard quotations summary
- `kaseya-bms-cli crm get-line-item`  -  Get line item
- `kaseya-bms-cli crm get-next-opportunities`  -  Get next opportunities
- `kaseya-bms-cli crm get-open-pipelines`  -  Get open pipelines
- `kaseya-bms-cli crm get-opportunity`  -  Get opportunity
- `kaseya-bms-cli crm get-opportunity-notes`  -  Get opportunity notes
- `kaseya-bms-cli crm get-opportunity-status-lookups`  -  Get opportunity status lookups
- `kaseya-bms-cli crm get-pipeline-totals`  -  Get pipeline totals
- `kaseya-bms-cli crm get-product-quotation`  -  Get product quotation
- `kaseya-bms-cli crm get-product-quotations-list-summary`  -  Get product quotations list summary
- `kaseya-bms-cli crm get-quotation`  -  Get quotation
- `kaseya-bms-cli crm get-quotations-list-summary`  -  Get quotations list summary
- `kaseya-bms-cli crm get-recurring-services-performance`  -  Get recurring services performance
- `kaseya-bms-cli crm get-recurring-services-performance-totals`  -  Get recurring services performance totals
- `kaseya-bms-cli crm get-sales-accelerator-data`  -  Get sales accelerator data
- `kaseya-bms-cli crm get-sales-accelerator-summary`  -  Get sales accelerator summary
- `kaseya-bms-cli crm get-sales-leaderboard`  -  Get sales leaderboard
- `kaseya-bms-cli crm get-top-opportunities`  -  Get top opportunities
- `kaseya-bms-cli crm patch-account`  -  Patch account
- `kaseya-bms-cli crm patch-contact`  -  Patch contact
- `kaseya-bms-cli crm post-account`  -  Post account
- `kaseya-bms-cli crm post-contact-summary-info`  -  Post contact summary info
- `kaseya-bms-cli crm post-opportunity`  -  Post opportunity
- `kaseya-bms-cli crm post-opportunity-note`  -  Post opportunity note
- `kaseya-bms-cli crm post-quotation-line-item`  -  Post quotation line item
- `kaseya-bms-cli crm put-account`  -  Put account
- `kaseya-bms-cli crm put-contact`  -  Put contact
- `kaseya-bms-cli crm put-opportunity`  -  Put opportunity
- `kaseya-bms-cli crm put-opportunity-note`  -  Put opportunity note
- `kaseya-bms-cli crm put-quotation-line-item`  -  Put quotation line item

**finance**  -  Manage finance

- `kaseya-bms-cli finance activate-contract`  -  Activate contract
- `kaseya-bms-cli finance deactivate-contract`  -  Deactivate contract
- `kaseya-bms-cli finance get-contract-lookups`  -  Get contract lookups
- `kaseya-bms-cli finance get-contracts-summary`  -  Get contracts summary
- `kaseya-bms-cli finance get-invoice`  -  Get invoice
- `kaseya-bms-cli finance get-invoice-details-list`  -  Get invoice details list
- `kaseya-bms-cli finance get-invoice-discounts-list`  -  Get invoice discounts list
- `kaseya-bms-cli finance get-invoices-summary-list`  -  Get invoices summary list
- `kaseya-bms-cli finance mark-invoices-as-sent`  -  Mark invoices as sent
- `kaseya-bms-cli finance post-recurring-service-contract`  -  Post recurring service contract
- `kaseya-bms-cli finance post-recurring-services`  -  Post recurring services
- `kaseya-bms-cli finance put-recurring-service-contract`  -  Put recurring service contract

**hr**  -  Manage hr

- `kaseya-bms-cli hr get-assignee-lookups`  -  Get assignee lookups
- `kaseya-bms-cli hr get-assignees`  -  Get assignees
- `kaseya-bms-cli hr get-assignees-count`  -  Get assignees count
- `kaseya-bms-cli hr get-employee-role-lookups`  -  Get employee role lookups
- `kaseya-bms-cli hr get-employees-list-search-select`  -  Get employees list search select
- `kaseya-bms-cli hr get-employees-lookup`  -  Get employees lookup

**integration**  -  Manage integration

- `kaseya-bms-cli integration get-account-codes`  -  Get account codes
- `kaseya-bms-cli integration get-account-codes-by-ids`  -  Get account codes by ids
- `kaseya-bms-cli integration get-bill-details`  -  Get bill details
- `kaseya-bms-cli integration get-bills`  -  Get bills
- `kaseya-bms-cli integration get-class-lists`  -  Get class lists
- `kaseya-bms-cli integration get-class-lists-by-ids`  -  Get class lists by ids
- `kaseya-bms-cli integration get-client-by-ids`  -  Get client by ids
- `kaseya-bms-cli integration get-clients`  -  Get clients
- `kaseya-bms-cli integration get-discounts`  -  Get discounts
- `kaseya-bms-cli integration get-discounts-by-ids`  -  Get discounts by ids
- `kaseya-bms-cli integration get-expenses`  -  Get expenses
- `kaseya-bms-cli integration get-expenses-by-ids`  -  Get expenses by ids
- `kaseya-bms-cli integration get-invoice-details`  -  Get invoice details
- `kaseya-bms-cli integration get-invoices`  -  Get invoices
- `kaseya-bms-cli integration get-job-info`  -  Get job info
- `kaseya-bms-cli integration get-products`  -  Get products
- `kaseya-bms-cli integration get-products-by-ids`  -  Get products by ids
- `kaseya-bms-cli integration get-reimbursement-details`  -  Get reimbursement details
- `kaseya-bms-cli integration get-reimbursements`  -  Get reimbursements
- `kaseya-bms-cli integration get-sales-tax-items`  -  Get sales tax items
- `kaseya-bms-cli integration get-sales-tax-items-by-ids`  -  Get sales tax items by ids
- `kaseya-bms-cli integration get-services`  -  Get services
- `kaseya-bms-cli integration get-services-by-ids`  -  Get services by ids
- `kaseya-bms-cli integration get-vendors`  -  Get vendors
- `kaseya-bms-cli integration get-vendors-by-ids`  -  Get vendors by ids
- `kaseya-bms-cli integration get-work-types`  -  Get work types
- `kaseya-bms-cli integration get-work-types-by-ids`  -  Get work types by ids
- `kaseya-bms-cli integration import-account-codes`  -  Import account codes
- `kaseya-bms-cli integration import-class-lists`  -  Import class lists
- `kaseya-bms-cli integration import-clients`  -  Import clients
- `kaseya-bms-cli integration import-discounts`  -  Import discounts
- `kaseya-bms-cli integration import-expenses`  -  Import expenses
- `kaseya-bms-cli integration import-payments`  -  Import payments
- `kaseya-bms-cli integration import-products`  -  Import products
- `kaseya-bms-cli integration import-services`  -  Import services
- `kaseya-bms-cli integration import-vendors`  -  Import vendors
- `kaseya-bms-cli integration import-work-types`  -  Import work types
- `kaseya-bms-cli integration post-entity-mappings`  -  Post entity mappings
- `kaseya-bms-cli integration update-bill-qb-reference`  -  Update bill qb reference
- `kaseya-bms-cli integration update-invoice-qb-reference`  -  Update invoice qb reference
- `kaseya-bms-cli integration update-reimbursement-qb-reference`  -  Update reimbursement qb reference

**integrations**  -  Manage integrations

- `kaseya-bms-cli integrations get-distributors-pricing`  -  Get distributors pricing
- `kaseya-bms-cli integrations get-distributors-status`  -  Get distributors status
- `kaseya-bms-cli integrations get-etilize-product`  -  Get etilize product
- `kaseya-bms-cli integrations get-itg-account-lookup`  -  Get itg account lookup
- `kaseya-bms-cli integrations get-itg-asset-lookup`  -  Get itg asset lookup
- `kaseya-bms-cli integrations get-itg-checklist-tasks`  -  Get itg checklist tasks
- `kaseya-bms-cli integrations get-itg-checklists`  -  Get itg checklists
- `kaseya-bms-cli integrations get-itg-contact-lookup`  -  Get itg contact lookup
- `kaseya-bms-cli integrations get-itg-contact-notes`  -  Get itg contact notes
- `kaseya-bms-cli integrations get-itg-location-lookup`  -  Get itg location lookup
- `kaseya-bms-cli integrations get-itg-organization-notes`  -  Get itg organization notes
- `kaseya-bms-cli integrations get-itg-status`  -  Get itg status
- `kaseya-bms-cli integrations get-itgaccess-info`  -  Get itgaccess info
- `kaseya-bms-cli integrations get-itgpassword-value`  -  Get itgpassword value
- `kaseya-bms-cli integrations get-itgsuggested-resources`  -  Get itgsuggested resources
- `kaseya-bms-cli integrations get-itgsuggested-resources-count`  -  Get itgsuggested resources count
- `kaseya-bms-cli integrations get-survey-settings`  -  Get survey settings
- `kaseya-bms-cli integrations get-ticket-global-search`  -  Get ticket global search
- `kaseya-bms-cli integrations get-ticket-sync`  -  Get ticket sync
- `kaseya-bms-cli integrations get-tickets-sync`  -  Get tickets sync
- `kaseya-bms-cli integrations get-vsa-access-info`  -  Get vsa access info
- `kaseya-bms-cli integrations put-itgcontact-notes`  -  Put itgcontact notes
- `kaseya-bms-cli integrations put-itgorganization-notes`  -  Put itgorganization notes
- `kaseya-bms-cli integrations search-products`  -  Search products

**inventory**  -  Manage inventory

- `kaseya-bms-cli inventory get-polookups`  -  Get polookups
- `kaseya-bms-cli inventory get-pricing-levels`  -  Get pricing levels
- `kaseya-bms-cli inventory get-product-categories-lookup`  -  Get product categories lookup
- `kaseya-bms-cli inventory get-product-default-cost`  -  Get product default cost
- `kaseya-bms-cli inventory get-product-weighted-cost`  -  Get product weighted cost
- `kaseya-bms-cli inventory get-products-in-stock`  -  Get products in stock
- `kaseya-bms-cli inventory get-products-list-search-select`  -  Get products list search select
- `kaseya-bms-cli inventory get-supplier-lookups`  -  Get supplier lookups
- `kaseya-bms-cli inventory get-warehouse-lookups`  -  Get warehouse lookups
- `kaseya-bms-cli inventory post-product-categories`  -  Post product categories

**kaseya-one**  -  Manage kaseya one

- `kaseya-bms-cli kaseya-one get-k1-resource-lookup`  -  Get k1 resource lookup
- `kaseya-bms-cli kaseya-one get-k1-resources-lookup`  -  Get k1 resources lookup
- `kaseya-bms-cli kaseya-one get-k1-role-lookup`  -  Get k1 role lookup
- `kaseya-bms-cli kaseya-one get-k1-roles-lookup`  -  Get k1 roles lookup
- `kaseya-bms-cli kaseya-one get-k1-ticket-statuses-lookup`  -  Get k1 ticket statuses lookup
- `kaseya-bms-cli kaseya-one get-k1-ticket-statuses-lookup-kaseyaone`  -  Get k1 ticket statuses lookup kaseyaone
- `kaseya-bms-cli kaseya-one get-work-type-lookup`  -  Get work type lookup
- `kaseya-bms-cli kaseya-one get-work-types-lookup`  -  Get work types lookup
- `kaseya-bms-cli kaseya-one k1-get-users-list`  -  K1 get users list
- `kaseya-bms-cli kaseya-one post-k1-centralized-configurations`  -  Post k1 centralized configurations
- `kaseya-bms-cli kaseya-one post-k1-gorgon-provisioning-notifications`  -  Post k1 gorgon provisioning notifications
- `kaseya-bms-cli kaseya-one post-k1-ticket-action-notifications`  -  Post k1 ticket action notifications
- `kaseya-bms-cli kaseya-one post-k1-user-deprovisioning-notifications`  -  Post k1 user deprovisioning notifications

**listing**  -  Manage listing

- `kaseya-bms-cli listing get-columns`  -  Get columns
- `kaseya-bms-cli listing get-search-columns`  -  Get search columns
- `kaseya-bms-cli listing get-search-combo-items`  -  Get search combo items

**my**  -  Manage my

- `kaseya-bms-cli my get-expense-sheets-lookups`  -  Get expense sheets lookups
- `kaseya-bms-cli my post-expense-sheet`  -  Post expense sheet

**project**  -  Manage project

- `kaseya-bms-cli project delete-status`  -  Delete status
- `kaseya-bms-cli project get`  -  Get
- `kaseya-bms-cli project get-list-summary`  -  Get list summary
- `kaseya-bms-cli project get-lookup`  -  Get lookup
- `kaseya-bms-cli project get-status`  -  Get status
- `kaseya-bms-cli project get-statuses-list`  -  Get statuses list
- `kaseya-bms-cli project get-statuses-lookup`  -  Get statuses lookup
- `kaseya-bms-cli project get-task-related-items`  -  Get task related items
- `kaseya-bms-cli project get-tasks-list`  -  Get tasks list
- `kaseya-bms-cli project post-status`  -  Post status
- `kaseya-bms-cli project put-status`  -  Put status
- `kaseya-bms-cli project put-status-order`  -  Put status order
- `kaseya-bms-cli project search-and-select-task-list`  -  Search and select task list

**rmm**  -  Manage rmm

- `kaseya-bms-cli rmm create`  -  Create
- `kaseya-bms-cli rmm get`  -  Get
- `kaseya-bms-cli rmm get-rmmsettings`  -  Get rmmsettings
- `kaseya-bms-cli rmm post-alert`  -  Post alert
- `kaseya-bms-cli rmm update`  -  Update
- `kaseya-bms-cli rmm update-integrationtype`  -  Update integrationtype

**security**  -  Manage security

- `kaseya-bms-cli security authenticate`  -  Authenticate
- `kaseya-bms-cli security refresh-token`  -  Refresh token
- `kaseya-bms-cli security sso-status`  -  Sso status
- `kaseya-bms-cli security user-info`  -  User info

**servicedesk**  -  Manage servicedesk

- `kaseya-bms-cli servicedesk add-related-tickets`  -  Add related tickets
- `kaseya-bms-cli servicedesk add-ticket-tasks`  -  Add ticket tasks
- `kaseya-bms-cli servicedesk assign-ticket`  -  Assign ticket
- `kaseya-bms-cli servicedesk create`  -  Create
- `kaseya-bms-cli servicedesk create-tickets`  -  Create tickets
- `kaseya-bms-cli servicedesk create-tickets-2`  -  Create tickets 2
- `kaseya-bms-cli servicedesk create-tickets-3`  -  Create tickets 3
- `kaseya-bms-cli servicedesk create-tickets-4`  -  Create tickets 4
- `kaseya-bms-cli servicedesk delete-hardware-asset`  -  Delete hardware asset
- `kaseya-bms-cli servicedesk delete-related-ticket`  -  Delete related ticket
- `kaseya-bms-cli servicedesk delete-service-call`  -  Delete service call
- `kaseya-bms-cli servicedesk delete-service-calls-and-to-dos`  -  Delete service calls and to dos
- `kaseya-bms-cli servicedesk delete-ticket`  -  Delete ticket
- `kaseya-bms-cli servicedesk delete-ticket-charge`  -  Delete ticket charge
- `kaseya-bms-cli servicedesk delete-ticket-checklist-item`  -  Delete ticket checklist item
- `kaseya-bms-cli servicedesk delete-ticket-checklist-items`  -  Delete ticket checklist items
- `kaseya-bms-cli servicedesk delete-ticket-expense`  -  Delete ticket expense
- `kaseya-bms-cli servicedesk delete-ticket-note`  -  Delete ticket note
- `kaseya-bms-cli servicedesk delete-ticket-tasks`  -  Delete ticket tasks
- `kaseya-bms-cli servicedesk delete-ticket-time-entry`  -  Delete ticket time entry
- `kaseya-bms-cli servicedesk delete-tickets`  -  Delete tickets
- `kaseya-bms-cli servicedesk delete-to-do`  -  Delete to do
- `kaseya-bms-cli servicedesk get`  -  Get
- `kaseya-bms-cli servicedesk get-agent-procedure-audits`  -  Get agent procedure audits
- `kaseya-bms-cli servicedesk get-batch-action-logs`  -  Get batch action logs
- `kaseya-bms-cli servicedesk get-ccs`  -  Get ccs
- `kaseya-bms-cli servicedesk get-device-status`  -  Get device status
- `kaseya-bms-cli servicedesk get-hardware-asset`  -  Get hardware asset
- `kaseya-bms-cli servicedesk get-hardware-asset-tickets`  -  Get hardware asset tickets
- `kaseya-bms-cli servicedesk get-hardware-assets-list-search-select`  -  Get hardware assets list search select
- `kaseya-bms-cli servicedesk get-my-tickets`  -  Get my tickets
- `kaseya-bms-cli servicedesk get-product-charge-delivery-history-by-charge-id`  -  Get product charge delivery history by charge id
- `kaseya-bms-cli servicedesk get-queue-tickets`  -  Get queue tickets
- `kaseya-bms-cli servicedesk get-related-alerts`  -  Get related alerts
- `kaseya-bms-cli servicedesk get-related-tickets`  -  Get related tickets
- `kaseya-bms-cli servicedesk get-related-tickets-count`  -  Get related tickets count
- `kaseya-bms-cli servicedesk get-service-call`  -  Get service call
- `kaseya-bms-cli servicedesk get-service-calls`  -  Get service calls
- `kaseya-bms-cli servicedesk get-software-assets-list-search-select`  -  Get software assets list search select
- `kaseya-bms-cli servicedesk get-ticket`  -  Get ticket
- `kaseya-bms-cli servicedesk get-ticket-activities`  -  Get ticket activities
- `kaseya-bms-cli servicedesk get-ticket-charge`  -  Get ticket charge
- `kaseya-bms-cli servicedesk get-ticket-charge-product-summary`  -  Get ticket charge product summary
- `kaseya-bms-cli servicedesk get-ticket-checklist-items-by-ticket-id`  -  Get ticket checklist items by ticket id
- `kaseya-bms-cli servicedesk get-ticket-expense`  -  Get ticket expense
- `kaseya-bms-cli servicedesk get-ticket-expenses-charges`  -  Get ticket expenses charges
- `kaseya-bms-cli servicedesk get-ticket-logs`  -  Get ticket logs
- `kaseya-bms-cli servicedesk get-ticket-note`  -  Get ticket note
- `kaseya-bms-cli servicedesk get-ticket-note-template-details-by-ticket-id`  -  Get ticket note template details by ticket id
- `kaseya-bms-cli servicedesk get-ticket-note-template-lookups`  -  Get ticket note template lookups
- `kaseya-bms-cli servicedesk get-ticket-notes`  -  Get ticket notes
- `kaseya-bms-cli servicedesk get-ticket-product-charge-assets-by-charge-id`  -  Get ticket product charge assets by charge id
- `kaseya-bms-cli servicedesk get-ticket-service-calls-to-dos`  -  Get ticket service calls to dos
- `kaseya-bms-cli servicedesk get-ticket-slainfo`  -  Get ticket slainfo
- `kaseya-bms-cli servicedesk get-ticket-survey-scores`  -  Get ticket survey scores
- `kaseya-bms-cli servicedesk get-ticket-tabs-indicators`  -  Get ticket tabs indicators
- `kaseya-bms-cli servicedesk get-ticket-tasks`  -  Get ticket tasks
- `kaseya-bms-cli servicedesk get-ticket-tasks-count`  -  Get ticket tasks count
- `kaseya-bms-cli servicedesk get-ticket-template`  -  Get ticket template
- `kaseya-bms-cli servicedesk get-ticket-template-lookups`  -  Get ticket template lookups
- `kaseya-bms-cli servicedesk get-ticket-time-entries`  -  Get ticket time entries
- `kaseya-bms-cli servicedesk get-ticket-time-entry`  -  Get ticket time entry
- `kaseya-bms-cli servicedesk get-ticket-time-entry-template-lookups`  -  Get ticket time entry template lookups
- `kaseya-bms-cli servicedesk get-ticket-timelog-template-details-by-ticket-id`  -  Get ticket timelog template details by ticket id
- `kaseya-bms-cli servicedesk get-tickets`  -  Get tickets
- `kaseya-bms-cli servicedesk get-tickets-2`  -  Get tickets 2
- `kaseya-bms-cli servicedesk get-tickets-count`  -  Get tickets count
- `kaseya-bms-cli servicedesk get-tickets-count-by-assignee-priority`  -  Get tickets count by assignee priority
- `kaseya-bms-cli servicedesk get-tickets-count-by-basic-status`  -  Get tickets count by basic status
- `kaseya-bms-cli servicedesk get-tickets-count-by-issue-type`  -  Get tickets count by issue type
- `kaseya-bms-cli servicedesk get-tickets-count-by-priority`  -  Get tickets count by priority
- `kaseya-bms-cli servicedesk get-tickets-count-by-queue`  -  Get tickets count by queue
- `kaseya-bms-cli servicedesk get-tickets-count-by-status`  -  Get tickets count by status
- `kaseya-bms-cli servicedesk get-tickets-due`  -  Get tickets due
- `kaseya-bms-cli servicedesk get-tickets-list-search-select`  -  Get tickets list search select
- `kaseya-bms-cli servicedesk get-tickets-list-summary`  -  Get tickets list summary
- `kaseya-bms-cli servicedesk get-tickets-upcoming`  -  Get tickets upcoming
- `kaseya-bms-cli servicedesk get-time-logged-by-technician`  -  Get time logged by technician
- `kaseya-bms-cli servicedesk get-to-do`  -  Get to do
- `kaseya-bms-cli servicedesk get-vsa-hardware-asset-tickets`  -  Get vsa hardware asset tickets
- `kaseya-bms-cli servicedesk get-workflow-logs`  -  Get workflow logs
- `kaseya-bms-cli servicedesk merge-tickets`  -  Merge tickets
- `kaseya-bms-cli servicedesk patch-ticket`  -  Patch ticket
- `kaseya-bms-cli servicedesk patch-tickets`  -  Patch tickets
- `kaseya-bms-cli servicedesk post-checklist-tasks`  -  Post checklist tasks
- `kaseya-bms-cli servicedesk post-checklist-with-tasks`  -  Post checklist with tasks
- `kaseya-bms-cli servicedesk post-hardware-asset`  -  Post hardware asset
- `kaseya-bms-cli servicedesk post-po`  -  Post po
- `kaseya-bms-cli servicedesk post-product-deliver`  -  Post product deliver
- `kaseya-bms-cli servicedesk post-service-call`  -  Post service call
- `kaseya-bms-cli servicedesk post-ticket`  -  Post ticket
- `kaseya-bms-cli servicedesk post-ticket-charge`  -  Post ticket charge
- `kaseya-bms-cli servicedesk post-ticket-checklist-item`  -  Post ticket checklist item
- `kaseya-bms-cli servicedesk post-ticket-expense`  -  Post ticket expense
- `kaseya-bms-cli servicedesk post-ticket-note`  -  Post ticket note
- `kaseya-bms-cli servicedesk post-ticket-time-entry`  -  Post ticket time entry
- `kaseya-bms-cli servicedesk post-to-do`  -  Post to do
- `kaseya-bms-cli servicedesk put-hardware-asset`  -  Put hardware asset
- `kaseya-bms-cli servicedesk put-service-call`  -  Put service call
- `kaseya-bms-cli servicedesk put-ticket`  -  Put ticket
- `kaseya-bms-cli servicedesk put-ticket-charge`  -  Put ticket charge
- `kaseya-bms-cli servicedesk put-ticket-checklist-item`  -  Put ticket checklist item
- `kaseya-bms-cli servicedesk put-ticket-expense`  -  Put ticket expense
- `kaseya-bms-cli servicedesk put-ticket-note`  -  Put ticket note
- `kaseya-bms-cli servicedesk put-ticket-time-entry`  -  Put ticket time entry
- `kaseya-bms-cli servicedesk put-to-do`  -  Put to do
- `kaseya-bms-cli servicedesk resolve-ticket`  -  Resolve ticket
- `kaseya-bms-cli servicedesk search-my-tickets`  -  Search my tickets
- `kaseya-bms-cli servicedesk search-queue-tickets`  -  Search queue tickets
- `kaseya-bms-cli servicedesk search-tickets`  -  Search tickets
- `kaseya-bms-cli servicedesk update`  -  Update
- `kaseya-bms-cli servicedesk update-tickets`  -  Update tickets

**system**  -  Manage system

- `kaseya-bms-cli system create-tenant-lookup`  -  Create tenant lookup
- `kaseya-bms-cli system delete-attachment`  -  Delete attachment
- `kaseya-bms-cli system delete-attachments`  -  Delete attachments
- `kaseya-bms-cli system delete-temp-attachments`  -  Delete temp attachments
- `kaseya-bms-cli system delete-view`  -  Delete view
- `kaseya-bms-cli system download-attachment`  -  Download attachment
- `kaseya-bms-cli system download-profile-picture`  -  Download profile picture
- `kaseya-bms-cli system get-account-types-lookup`  -  Get account types lookup
- `kaseya-bms-cli system get-all-batch-action-logs`  -  Get all batch action logs
- `kaseya-bms-cli system get-approval-routes-lookup`  -  Get approval routes lookup
- `kaseya-bms-cli system get-attachment`  -  Get attachment
- `kaseya-bms-cli system get-attachments`  -  Get attachments
- `kaseya-bms-cli system get-colors-lookup`  -  Get colors lookup
- `kaseya-bms-cli system get-custom-fields-data`  -  Get custom fields data
- `kaseya-bms-cli system get-custom-fields-meta-data`  -  Get custom fields meta data
- `kaseya-bms-cli system get-email-template-lookups`  -  Get email template lookups
- `kaseya-bms-cli system get-expense-type-lookups`  -  Get expense type lookups
- `kaseya-bms-cli system get-issue-sub-type-lookups`  -  Get issue sub type lookups
- `kaseya-bms-cli system get-issue-type-lookups`  -  Get issue type lookups
- `kaseya-bms-cli system get-locations-lookup`  -  Get locations lookup
- `kaseya-bms-cli system get-lookups`  -  Get lookups
- `kaseya-bms-cli system get-priority-lookups`  -  Get priority lookups
- `kaseya-bms-cli system get-queue-lookups`  -  Get queue lookups
- `kaseya-bms-cli system get-role-lookups`  -  Get role lookups
- `kaseya-bms-cli system get-scheduler-job-status`  -  Get scheduler job status
- `kaseya-bms-cli system get-settings`  -  Get settings
- `kaseya-bms-cli system get-slalookups`  -  Get slalookups
- `kaseya-bms-cli system get-status-lookups`  -  Get status lookups
- `kaseya-bms-cli system get-tenant-lookups`  -  Get tenant lookups
- `kaseya-bms-cli system get-view-details-values`  -  Get view details values
- `kaseya-bms-cli system get-view-search-values`  -  Get view search values
- `kaseya-bms-cli system get-view-share-settings`  -  Get view share settings
- `kaseya-bms-cli system get-views`  -  Get views
- `kaseya-bms-cli system get-work-type-lookups`  -  Get work type lookups
- `kaseya-bms-cli system post`  -  Post
- `kaseya-bms-cli system post-event-log`  -  Post event log
- `kaseya-bms-cli system post-temp-attachment`  -  Post temp attachment
- `kaseya-bms-cli system post-view`  -  Post view
- `kaseya-bms-cli system put-attachment`  -  Put attachment
- `kaseya-bms-cli system put-custom-field-data`  -  Put custom field data
- `kaseya-bms-cli system put-view`  -  Put view
- `kaseya-bms-cli system put-view-info`  -  Put view info
- `kaseya-bms-cli system put-view-share-settings`  -  Put view share settings

**timelogs**  -  Manage timelogs

- `kaseya-bms-cli timelogs`  -  Get all time logs


**contract-burn**


- `kaseya-bms-cli contract-burn`  -  Per-contract burn picture: hours consumed, open tickets, and how much of the contract period has elapsed - at-risk agreements surface first.


**pipeline**


- `kaseya-bms-cli pipeline`  -  Open opportunities grouped by stage with counts, total and weighted value, and slipped-close flags for the Monday sales call.


**queue-health**


- `kaseya-bms-cli queue-health`  -  See open ticket volume by queue, priority, and status in one shot, with stale counts flagged before the morning standup.


**stale-tickets**


- `kaseya-bms-cli stale-tickets`  -  List open tickets that have not been touched in N days, oldest first, with account and assignee so nothing rots in the queue.


**unbilled**


- `kaseya-bms-cli unbilled`  -  Billable, approved, not-yet-billed time grouped by account, in hours - the month-end ready-to-bill review without the Excel export.


**workload**


- `kaseya-bms-cli workload`  -  Open and in-progress ticket load per assignee, flagging who is overloaded and who has slack before you dispatch the next ticket.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
kaseya-bms-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

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

## Auth Setup

BMS uses short-lived JWTs. Set KASEYA_BMS_USERNAME, KASEYA_BMS_PASSWORD, and KASEYA_BMS_TENANT (your company name from My Settings in BMS), and every command exchanges them for a token on first use and again when it expires; the token is kept in the 0600 credential store (never in config.toml) and sent as a Bearer header. `kaseya-bms-cli auth login` checks the login up front. If you already have a JWT, set KASEYA_BMS_TOKEN directly (it wins over the login). Regional tenants set KASEYA_BMS_BASE_URL (https://api.bms.kaseya.com is the default; EMEA uses https://api.bmsemea.kaseya.com, APAC https://api.bmsapac.kaseya.com, legacy Vorex https://api.vorexlogin.kaseya.com). API users with MFA run `kaseya-bms-cli auth login --mfa-code <code>` (the automatic mint cannot answer MFA) and again when that token expires.

Run `kaseya-bms-cli doctor` to verify setup.

## Credential safety

- Every API request, the login request and the bearer token go only to the scheme, host and port of the API base URL: `KASEYA_BMS_BASE_URL` (default `https://api.bms.kaseya.com`; EMEA `https://api.bmsemea.kaseya.com`, APAC `https://api.bmsapac.kaseya.com`). The login is `POST /v2/security/authenticate` on that same host. A redirect to any other host is refused, so a credential never reaches another host.
- Any output, file or MCP result that would repeat `KASEYA_BMS_PASSWORD`, `KASEYA_BMS_TOKEN`, `KASEYA_BMS_BEARER_AUTH`, a minted token or a credential-named value from a response (an IT Glue password, an integration access token, a webhook secret) prints "[text withheld: it repeated a secret value this program holds]" in its place. Responses are never cached on disk, and credential-named fields are dropped before anything reaches the local store.
- Only `credentials.toml` (mode 0600) holds a credential. The password from the environment is never written to disk. A minted token is cached there, bound to the host and login it was minted for: another login mints its own, and a token is never sent to another region's host.
- `--dry-run` sends nothing and never mints a token. Over MCP, a call that changes Kaseya BMS returns a preview and sends only with `confirm: true`, and a tool argument can never name a file to read or write.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, sync, and `--deliver` paths:

- `--json`  -  one JSON document on stdout (sync progress events go to stderr)
- `--compact`  -  keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain`  -  tabular rows (collection envelopes unwrap to the row array). `--csv` prefixes `'` on a cell or header key that starts with `=`, `+`, `-`, `@`, tab, CR, or LF (fullwidth `＝＋－＠` too) so a spreadsheet does not run it as a formula; a plain signed number such as `-5` or `+2.5e3` is unchanged.
- `--quiet`  -  one identity value per row, no envelope

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  kaseya-bms-cli project get mock-value --agent --select Error,Result,Success
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit confirmation**  -  `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success, and use `--ignore-missing` only when a missing delete target should count as success

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set  -  piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Paths and state

Agents should treat the CLI's path resolver as part of the runtime contract:

- Use `--home <dir>` for one invocation, or set `KASEYA_BMS_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `KASEYA_BMS_CONFIG_DIR`, `KASEYA_BMS_DATA_DIR`, `KASEYA_BMS_STATE_DIR`, `KASEYA_BMS_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `KASEYA_BMS_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `kaseya-bms-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "kaseya-bms": {
        "command": "kaseya-bms-mcp",
        "env": {
          "KASEYA_BMS_HOME": "/srv/kaseya-bms"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `KASEYA_BMS_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `KASEYA_BMS_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are never cached on disk. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `KASEYA_BMS_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `KASEYA_BMS_DATA_SOURCE` sets the `--data-source` default.

## Automatic learning

This CLI ships a self-capturing learning loop. The CLI does its own bookkeeping: every invocation is journaled locally, a failed flag followed by a corrected retry auto-derives a `flag_alias` candidate, and a `teach` on a query family without a playbook auto-synthesizes a `playbook_candidate` from the session's journal. Your job is judgment only: `recall` first, act on surfaced candidates, `teach` the final answer, `playbook amend` when you observe a correction. You never record failures by hand.

### Step 1: `recall` before any discovery

Before list/search/drill commands on a new user question, pass the question as an argv or MCP tool argument to `recall --agent`. Do not interpolate user-controlled text into a shell command line.

Quoted `recall "<question>"` breaks on an apostrophe, which is ordinary English. A quoted heredoc breaks when a body line equals the delimiter, and that delimiter is published in these docs. Write the question with a non-shell file-writing tool, then read it back as data:

```bash
# Write the question verbatim with your file-writing tool (no shell involved).
# Command substitution on a file only ever yields data  -  the shell never
# parses the file's bytes as syntax.
QUERY=$(cat /path/to/question.txt)
kaseya-bms-cli recall "$QUERY" --agent
```

Prefer MCP: pass the question as the tool's query argument. `"$QUERY"` after a file read is argv-safe; putting the question itself in the command text is not.

The response envelope:

```json
{
  "query": "...",
  "normalized": "<normalized form>",
  "query_entities": ["..."],
  "found": true | false,
  "match_score": 0.0,
  "results": [
    { "resource_id": "...", "resource_type": "...", "venue": "...",
      "confidence": 2, "entity_match": "exact|partial|unknown",
      "source": "taught|preseed|pattern", "warnings": ["..."] }
  ],
  "mismatches": [ /* only when --debug-mismatches */ ],
  "warnings": [ /* top-level */ ],
  "candidates": [
    { "id": 12, "class": "flag_alias | playbook_candidate",
      "summary": "...", "sightings": 3, "last_seen": "...",
      "rationale": "...",
      "next_action": ["<trial command>", "kaseya-bms-cli learnings confirm 12"] }
  ],
  "playbook": {
    "query_family": "...",
    "playbook": {
      "steps": [ { "cmd": "<command with {slot} substitution>", "purpose": "..." } ],
      "entity_slots": ["$ENTITY"],
      "expected_tool_calls": 3
    },
    "slots_resolved": { "$ENTITY": { "token": "<live token>", "canonical": "<canonical>" } },
    "notes": "<workarounds + gotchas for this query family>"
  },
  "notes": "<duplicate surface for non-playbook callers>"
}
```

Empty-store short-circuit: if the store has no learnings, playbooks, or candidates yet (recall finds nothing and `learnings list` and `learnings candidates` are both empty), skip recall for the rest of this session instead of taxing every query; resume recall-first once something has been taught.

### Step 2: decision tree

Read `candidates`, `playbook`, `notes`, `results[0]`, and warnings in that order:

```
if Candidates present (warnings include "candidates_present"):
    -> candidates are try-then-confirm, never facts. Follow each candidate's
       two-step next_action verbatim: run the trial command first, then run
       `learnings confirm <id>` only after the trial verified the behavior.
       Reject a wrong candidate with `learnings reject <id>`.
    -> NEVER re-teach something recall surfaced as a candidate; confirm or
       reject that candidate instead of teaching a duplicate.
    -> candidates ride alongside playbooks and resource hits, not instead of
       them; continue with the branches below after acting on them.

if Playbook present:
    -> READ Playbook.notes verbatim FIRST (workarounds + gotchas the CLI surface doesn't expose)
    -> replay Playbook.steps in order, substituting Playbook.slots_resolved entries
       for the entity slot tokens. If a step's slot is unresolved, fall back to
       discovery for that step only.
    -> the Playbook's expected_tool_calls is a budget; if you find yourself running
       materially more, record the divergence via `kaseya-bms-cli playbook amend`
       at end-of-session.

elif Notes present (no Playbook):
    -> read Notes verbatim before any discovery step; they carry known gotchas
       for this query family even when no structured choreography exists yet.

elif Found AND Results[0].EntityMatch == "exact" AND Results[0].Confidence >= 2:
    -> skip discovery; fetch live data for Results[*].ResourceID in parallel

elif Found AND Results[0].EntityMatch == "partial":
    -> candidate hint, NOT a hit; read the resource title to validate before trusting

elif (any row in Mismatches[] when --debug-mismatches was passed):
    -> treat as cold start; the stored learning is for a different entity
       (different canonical resolved from query_entities)

else:  // Found == false, no playbook, no notes
    -> cold start; run discovery normally; teach the answer afterward (Step 4).
       If the family has no playbook yet, that teach auto-synthesizes a
       playbook candidate from this session's journal - you do not need to
       record one by hand.
```

Playbook and Notes are orthogonal to the per-resource path. A recall response can carry both a Playbook AND a `Results[]` hit - use both: the Playbook tells you which choreography to run; the resource hits short-circuit specific steps. Default to skipping `mismatches`; pass `--debug-mismatches` only when investigating cold-start surprises.

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `kaseya-bms-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `kaseya-bms-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
kaseya-bms-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
kaseya-bms-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
kaseya-bms-cli teach-playbook \
  --query "$QUERY" \
  --playbook-file ~/playbooks/<shape>.json \
  --notes-file ~/playbooks/<shape>-notes.md
```

Playbook files are JSON with `steps`, `entity_slots`, `expected_tool_calls`. Notes files are markdown carrying the gotchas verbatim. File-free callers (MCP-only agents) pass the same content inline: `--playbook-json` and `--playbook-notes` on the integrated `teach` form, `--playbook-json` and `--notes` on `teach-playbook`. On the integrated `teach` form, the playbook flags are optional - omit them entirely for a resource-only teach. On the standalone `teach-playbook` form, at least one of the playbook and notes flags must be set; both empty is rejected. Playbooks are keyed on the structural query family (entities stripped) so a recipe taught from one entity-shaped query applies to every other query of the same shape, with `slots_resolved` binding the live query's canonical at recall time.

When you DO find a playbook on a future recall, treat it as ground truth: replay the steps with `slots_resolved` substitutions, skip the discovery that the choreography already documents, and read `notes` before any step.

### Step 6: `playbook amend &` when your debug response identifies a correction

If your debug-protocol response identifies a concrete correction the notes or playbook should know  -  a workaround, an undocumented endpoint shape, a stale field name, observed schema drift, an empty-payload fallback  -  fire `playbook amend` BEFORE emitting your user-facing response. Same fire-and-forget posture as `teach`. Pass the query and note as argv/MCP arguments, or write each with a non-shell file tool and read them back (`QUERY=$(cat ...)`, `NOTE=$(cat ...)`). Do not interpolate either string into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
NOTE=$(cat /path/to/note.txt)
kaseya-bms-cli playbook amend \
  --query "$QUERY" \
  --add-note "$NOTE"
# (append shell `&` to background it)
```

What counts as worth amending: a behavior you OBSERVED this session that future-you would benefit from knowing. Examples worth amending:

- A workaround for a CLI surface that silently drops or misorders a flag.
- An undocumented endpoint shape (response wrapped in `{meta, results}`, payload nested two levels deeper than the docs claim).
- Observed schema drift (a field renamed, an index that shifted between seasons, a category label that the API now returns lower-cased).

What does NOT belong in notes:

- The year-specific or entity-specific answer to the user's question. That's the response, not a learning.
- Per-team / per-athlete / per-row data the playbook already retrieves at runtime.
- Statements that paraphrase what the existing notes already say.

The amend command appends to the family's existing notes with a timestamped marker (`[amend YYYY-MM-DDTHH:MMZ]: <text>`). Multiple amends accumulate; the audit trail is visible. If no playbook exists yet for the family, amend creates a notes-only one (so cold-start corrections still land).

#### PII discipline for amend notes

`playbook amend` notes are designed to potentially flow upstream as shared knowledge in future versions of the Printing Press. Keep them clean of user-identifying content so the upstream-contribution path stays open without retroactive scrubbing:

- **Do NOT embed** paths to user filesystems, personal API keys or tokens, user email addresses, user GitHub handles, or specific query histories tied to a single user.
- **Acceptable**: endpoint shapes, undocumented field names, API gotchas, observed schema drift, workarounds for CLI surfaces, generalizable pagination or retry tactics.

If a correction is only meaningful with user-specific context, it belongs in a personal note, not in the playbook amend.

### Measuring the loop

`kaseya-bms-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `KASEYA_BMS_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
kaseya-bms-cli feedback "the --since flag is inclusive but docs say exclusive"
kaseya-bms-cli feedback --stdin < notes.txt
kaseya-bms-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `KASEYA_BMS_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `KASEYA_BMS_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` as a private 0600 file (exclusive randomly named temp in the destination directory, then rename; a planted symlink is not followed). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`). Redirects are not followed; a 3xx response fails the delivery and the body is not sent to the `Location`. |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures, including redirects, return non-zero and log the URL and HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
kaseya-bms-cli profile save briefing --json
kaseya-bms-cli --profile briefing project get mock-value
kaseya-bms-cli profile list --json
kaseya-bms-cli profile show briefing
kaseya-bms-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Async Jobs

For endpoints that submit long-running work, the generator detects the submit-then-poll pattern (a `job_id`/`task_id`/`operation_id` field in the response plus a sibling status endpoint) and wires up three extra flags on the submitting command:

| Flag | Purpose |
|------|---------|
| `--wait` | Block until the job reaches a terminal status instead of returning the job ID immediately |
| `--wait-timeout` | Maximum wait duration (default 10m, 0 means no timeout) |
| `--wait-interval` | Initial poll interval (default 2s; grows with exponential backoff up to 30s) |

Use async submission without `--wait` when you want to fire-and-forget; use `--wait` when you want one command to return the finished artifact.

If `--wait` times out or polling fails after the job was accepted, the command exits `8` and prints the job ID plus a recovery command that fetches the result. The job may still be running and may already be billed, so run the recovery command instead of resubmitting.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 6 | Partial failure |
| 7 | Rate limited (wait and retry) |
| 8 | Async job submitted but not finished (run the printed recovery command; do not resubmit) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `kaseya-bms-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add kaseya-bms-mcp -- kaseya-bms-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `kaseya-bms-mcp --transport http` requires `KASEYA_BMS_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `KASEYA_BMS_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `KASEYA_BMS_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which kaseya-bms-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   kaseya-bms-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `kaseya-bms-cli <command> --help`.
