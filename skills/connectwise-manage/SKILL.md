---
name: connectwise-manage
description: "Every ConnectWise PSA workflow from the terminal  -  with a typed conditions query builder, offline SQLite sync, and cross-entity views (unbilled work, account 360, board triage) the PSA web UI can't give you. Trigger phrases: `list connectwise tickets`, `triage the help desk board`, `find unbilled time in connectwise`, `connectwise account 360 for a client`, `log time on a connectwise ticket`, `use connectwise-manage`, `run connectwise-manage-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "ConnectWise Manage"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - connectwise-manage-cli
    install:
      - kind: go
        bins: [connectwise-manage-cli]
        module: github.com/mvanhorn/printing-press-library/library/project-management/connectwise-manage/cmd/connectwise-manage-cli
---

# ConnectWise PSA  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `connectwise-manage-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install connectwise-manage --cli-only
   ```
2. Verify: `connectwise-manage-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/project-management/connectwise-manage/cmd/connectwise-manage-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

A spec-generated CLI over the ConnectWise Manage REST API covering tickets, time, companies, contacts, agreements, configurations, projects, opportunities, and members. It syncs the high-gravity entities into a local SQLite store so you get instant full-text search and cross-table views the PSA never surfaces in one place  -  `unbilled` reconciles tickets against logged time, `account` assembles a full company 360, `board`/`stale`/`workload` give a dispatcher's queue at a glance. Every command speaks `--json`/`--select` and the whole tree is exposed as an MCP server for AI-driven triage.

## When to Use This CLI

Use this CLI when an MSP task touches the ConnectWise PSA system-of-record from the terminal or an agent: triaging or updating service tickets, logging billable time, pulling company / contact / agreement context, checking the CMDB, or reconciling unbilled work and agreement burn. It is the right choice when you want offline-fast queries, cross-entity views the web UI can't compose, or an MCP surface over the PSA. It is not the right choice for ConnectWise Automate (RMM) or for tenant administration screens the REST API doesn't cover.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Local joins that compound
- **`unbilled`**  -  Find tickets you touched or closed in a window that have zero or under-threshold time logged against them.

  _Reach for this before a billing cutoff or at end-of-day to catch revenue leaking from unlogged time._

  ```bash
  connectwise-manage-cli unbilled --since 7d --agent
  ```
- **`account`**  -  One card for a company: contacts, active agreements, deployed configurations, open-ticket count, and last activity.

  _Use it to prep a QBR or to get full context before any escalation call without clicking through tabs._

  ```bash
  connectwise-manage-cli account "Acme Corp" --agent
  ```
- **`agreement-burn`**  -  Hours logged against an agreement's company in a period versus the agreement's allotment, as a utilization percentage with over/under flag.

  _Use it to spot unprofitable clients before they blow their block-hours._

  ```bash
  connectwise-manage-cli agreement-burn --period 30d --agent
  ```

### Dispatcher views
- **`board`**  -  Open tickets on a board, oldest first, with each ticket's age, owner, status, and priority joined from the synced reference data.

  _Reach for this for the morning queue sweep instead of reloading the web board view._

  ```bash
  connectwise-manage-cli board 2 --unassigned
  ```
- **`stale`**  -  Open tickets with no update in N days, oldest first, with board and owner columns so you see what's rotting and whose it is.

  _Use it for the daily 'what's rotting on my board' pass before standup._

  ```bash
  connectwise-manage-cli stale --days 5
  ```
- **`workload`**  -  Open ticket count and aging per tech, so you route the next ticket to whoever is lightest.

  _Reach for this when deciding who should take a new escalation._

  ```bash
  connectwise-manage-cli workload --agent
  ```

### Query ergonomics
- **`condition`**  -  Build a validated ConnectWise conditions expression from flags (handling string quoting, bracketed dates, and AND-default / OR-parentheses), or explain what an existing expression queries.

  _Use it whenever a list command returns surprisingly empty  -  the DSL's quoting and AND/OR rules are the usual culprit._

  ```bash
  connectwise-manage-cli condition build --field board/name --op = --value "Help Desk"
  ```

## Command Reference

**company**  -  Manage company

- `connectwise-manage-cli company delete-companies-by-id`  -  Delete ConnectWise.Apis.v3_0.v2015_3.Company.Company.Company
- `connectwise-manage-cli company delete-companies-statuses-by-id`  -  Delete CompanyStatus
- `connectwise-manage-cli company delete-companies-types-by-id`  -  Delete Usage
- `connectwise-manage-cli company delete-configurations-bulk`  -  Delete BulkResult
- `connectwise-manage-cli company delete-configurations-by-id`  -  Delete Configuration
- `connectwise-manage-cli company delete-configurations-statuses-by-id`  -  Delete ConfigurationStatus
- `connectwise-manage-cli company delete-configurations-types-by-id`  -  Delete ConfigurationType
- `connectwise-manage-cli company delete-contacts-by-id`  -  Delete ApiContact
- `connectwise-manage-cli company delete-contacts-departments-by-id`  -  Delete Usage
- `connectwise-manage-cli company delete-contacts-relationships-by-id`  -  Delete ContactRelationship
- `connectwise-manage-cli company delete-contacts-types-by-id`  -  Delete ContactType
- `connectwise-manage-cli company get-companies`  -  Get List of ConnectWise.Apis.v3_0.v2015_3.Company.Company.Company
- `connectwise-manage-cli company get-companies-by-id`  -  Get ConnectWise.Apis.v3_0.v2015_3.Company.Company.Company
- `connectwise-manage-cli company get-companies-by-id-usages`  -  Get List of Usage Count
- `connectwise-manage-cli company get-companies-by-parent-id-custom-status-notes`  -  Get List of CompanyCustomNote
- `connectwise-manage-cli company get-companies-by-parent-id-groups`  -  Get List of CompanyGroup
- `connectwise-manage-cli company get-companies-by-parent-id-management-report-notifications`  -  Get List of ManagementReportNotification
- `connectwise-manage-cli company get-companies-by-parent-id-management-report-setup`  -  Get List of ManagementReportSetup
- `connectwise-manage-cli company get-companies-by-parent-id-management-summary-reports`  -  Get List of CompanyManagementSummary
- `connectwise-manage-cli company get-companies-by-parent-id-notes`  -  Get List of CompanyNote
- `connectwise-manage-cli company get-companies-by-parent-id-sites`  -  Get List of CompanySite
- `connectwise-manage-cli company get-companies-by-parent-id-teams`  -  Get List of CompanyTeam
- `connectwise-manage-cli company get-companies-by-parent-id-tracks`  -  Get List of ContactTrack
- `connectwise-manage-cli company get-companies-by-parent-id-type-associations`  -  Get List of CompanyTypeAssociation
- `connectwise-manage-cli company get-companies-count`  -  Get Count of ConnectWise.Apis.v3_0.v2015_3.Company.Company.Company
- `connectwise-manage-cli company get-companies-default`  -  Get ConnectWise.Apis.v3_0.v2015_3.Company.Company.Company
- `connectwise-manage-cli company get-companies-info`  -  Get List of CompanyInfos
- `connectwise-manage-cli company get-companies-info-count`  -  Get Count of CompanyInfos
- `connectwise-manage-cli company get-companies-info-types`  -  Get List of CompanyTypeInfo
- `connectwise-manage-cli company get-companies-statuses`  -  Get List of CompanyStatus
- `connectwise-manage-cli company get-companies-statuses-by-id`  -  Get CompanyStatus
- `connectwise-manage-cli company get-companies-statuses-count`  -  Get Count of CompanyStatus
- `connectwise-manage-cli company get-companies-types`  -  Get List of CompanyType
- `connectwise-manage-cli company get-companies-types-by-id`  -  Get CompanyType
- `connectwise-manage-cli company get-companies-types-count`  -  Get Count of CompanyType
- `connectwise-manage-cli company get-configurations`  -  Get List of Configuration
- `connectwise-manage-cli company get-configurations-by-id`  -  Get Configuration
- `connectwise-manage-cli company get-configurations-count`  -  Get Count of Configuration
- `connectwise-manage-cli company get-configurations-statuses`  -  Get List of ConfigurationStatus
- `connectwise-manage-cli company get-configurations-statuses-by-id`  -  Get ConfigurationStatus
- `connectwise-manage-cli company get-configurations-statuses-count`  -  Get Count of ConfigurationStatus
- `connectwise-manage-cli company get-configurations-statuses-info`  -  Get List of ConfigurationStatusInfos
- `connectwise-manage-cli company get-configurations-types`  -  Get List of ConfigurationType
- `connectwise-manage-cli company get-configurations-types-by-id`  -  Get ConfigurationType
- `connectwise-manage-cli company get-configurations-types-count`  -  Get Count of ConfigurationType
- `connectwise-manage-cli company get-contacts`  -  Get List of ApiContact
- `connectwise-manage-cli company get-contacts-by-id`  -  Get ApiContact
- `connectwise-manage-cli company get-contacts-by-id-image`  -  Get ValidatePortalResponse
- `connectwise-manage-cli company get-contacts-by-id-info`  -  Get ContactInfos
- `connectwise-manage-cli company get-contacts-by-id-portal-security`  -  Get List of PortalSecurity
- `connectwise-manage-cli company get-contacts-by-id-usages`  -  Get List of Usage Count
- `connectwise-manage-cli company get-contacts-by-parent-id-communications`  -  Get List of ContactCommunication
- `connectwise-manage-cli company get-contacts-by-parent-id-groups`  -  Get List of ContactGroup
- `connectwise-manage-cli company get-contacts-by-parent-id-notes`  -  Get List of ContactNote
- `connectwise-manage-cli company get-contacts-by-parent-id-tracks`  -  Get List of ContactTrack
- `connectwise-manage-cli company get-contacts-by-parent-id-type-associations`  -  Get List of ContactTypeAssociation
- `connectwise-manage-cli company get-contacts-count`  -  Get Count of Usage
- `connectwise-manage-cli company get-contacts-default`  -  Get ApiContact
- `connectwise-manage-cli company get-contacts-departments`  -  Get List of ContactDepartment
- `connectwise-manage-cli company get-contacts-departments-by-id`  -  Get ContactDepartment
- `connectwise-manage-cli company get-contacts-departments-count`  -  Get Count of ContactDepartment
- `connectwise-manage-cli company get-contacts-departments-info`  -  Get List of ContactDepartmentInfos
- `connectwise-manage-cli company get-contacts-info`  -  Get List of ContactInfos
- `connectwise-manage-cli company get-contacts-info-count`  -  Get Count of ContactInfos
- `connectwise-manage-cli company get-contacts-relationships`  -  Get List of ContactRelationship
- `connectwise-manage-cli company get-contacts-relationships-by-id`  -  Get ContactRelationship
- `connectwise-manage-cli company get-contacts-relationships-count`  -  Get Count of ContactRelationship
- `connectwise-manage-cli company get-contacts-types`  -  Get List of ContactType
- `connectwise-manage-cli company get-contacts-types-by-id`  -  Get ContactType
- `connectwise-manage-cli company get-contacts-types-count`  -  Get Count of ContactType
- `connectwise-manage-cli company get-contacts-types-info`  -  Get List of ContactTypeInfo
- `connectwise-manage-cli company patch-companies-by-id`  -  Patch ConnectWise.Apis.v3_0.v2015_3.Company.Company.Company
- `connectwise-manage-cli company patch-companies-statuses-by-id`  -  Patch CompanyStatus
- `connectwise-manage-cli company patch-companies-types-by-id`  -  Patch CompanyType
- `connectwise-manage-cli company patch-configurations-by-id`  -  Patch Configuration
- `connectwise-manage-cli company patch-configurations-by-id-change-type`  -  Patch Configuration
- `connectwise-manage-cli company patch-configurations-statuses-by-id`  -  Patch ConfigurationStatus
- `connectwise-manage-cli company patch-configurations-types-by-id`  -  Patch ConfigurationType
- `connectwise-manage-cli company patch-contacts-by-id`  -  Patch ApiContact
- `connectwise-manage-cli company patch-contacts-departments-by-id`  -  Patch ContactDepartment
- `connectwise-manage-cli company patch-contacts-relationships-by-id`  -  Patch ContactRelationship
- `connectwise-manage-cli company patch-contacts-types-by-id`  -  Patch ContactType
- `connectwise-manage-cli company post-companies`  -  Post ConnectWise.Apis.v3_0.v2015_3.Company.Company.Company
- `connectwise-manage-cli company post-companies-by-id-merge`  -  Post SuccessResponse
- `connectwise-manage-cli company post-companies-by-parent-id-custom-status-notes`  -  Post CompanyCustomNote
- `connectwise-manage-cli company post-companies-by-parent-id-groups`  -  Post CompanyGroup
- `connectwise-manage-cli company post-companies-by-parent-id-management-report-notifications`  -  Post ManagementReportNotification
- `connectwise-manage-cli company post-companies-by-parent-id-management-report-setup`  -  Post ManagementReportSetup
- `connectwise-manage-cli company post-companies-by-parent-id-management-summary-reports`  -  Post CompanyManagementSummary
- `connectwise-manage-cli company post-companies-by-parent-id-notes`  -  Post CompanyNote
- `connectwise-manage-cli company post-companies-by-parent-id-sites`  -  Post CompanySite
- `connectwise-manage-cli company post-companies-by-parent-id-teams`  -  Post CompanyTeam
- `connectwise-manage-cli company post-companies-by-parent-id-tracks`  -  Post ContactTrack
- `connectwise-manage-cli company post-companies-by-parent-id-type-associations`  -  Post CompanyTypeAssociation
- `connectwise-manage-cli company post-companies-statuses`  -  Post CompanyStatus
- `connectwise-manage-cli company post-companies-types`  -  Post CompanyType
- `connectwise-manage-cli company post-configurations`  -  Post Configuration
- `connectwise-manage-cli company post-configurations-bulk`  -  Post Configuration
- `connectwise-manage-cli company post-configurations-statuses`  -  Post ConfigurationStatus
- `connectwise-manage-cli company post-configurations-types`  -  Post ConfigurationType
- `connectwise-manage-cli company post-configurations-types-copy`  -  Post Board
- `connectwise-manage-cli company post-contacts`  -  Post ApiContact
- `connectwise-manage-cli company post-contacts-by-parent-id-communications`  -  Post ContactCommunication
- `connectwise-manage-cli company post-contacts-by-parent-id-groups`  -  Post ContactGroup
- `connectwise-manage-cli company post-contacts-by-parent-id-notes`  -  Post ContactNote
- `connectwise-manage-cli company post-contacts-by-parent-id-tracks`  -  Post ContactTrack
- `connectwise-manage-cli company post-contacts-by-parent-id-type-associations`  -  Post ContactTypeAssociation
- `connectwise-manage-cli company post-contacts-departments`  -  Post ContactDepartment
- `connectwise-manage-cli company post-contacts-relationships`  -  Post ContactRelationship
- `connectwise-manage-cli company post-contacts-request-password`  -  Post PortalSecurity
- `connectwise-manage-cli company post-contacts-types`  -  Post ContactType
- `connectwise-manage-cli company post-contacts-validate-portal-credentials`  -  Post ValidatePortalResponse
- `connectwise-manage-cli company put-companies-by-id`  -  Put ConnectWise.Apis.v3_0.v2015_3.Company.Company.Company
- `connectwise-manage-cli company put-companies-statuses-by-id`  -  Put CompanyStatus
- `connectwise-manage-cli company put-companies-types-by-id`  -  Put CompanyType
- `connectwise-manage-cli company put-configurations-bulk`  -  Put Configuration
- `connectwise-manage-cli company put-configurations-by-id`  -  Put Configuration
- `connectwise-manage-cli company put-configurations-statuses-by-id`  -  Put ConfigurationStatus
- `connectwise-manage-cli company put-configurations-types-by-id`  -  Put ConfigurationType
- `connectwise-manage-cli company put-contacts-by-id`  -  Put ApiContact
- `connectwise-manage-cli company put-contacts-departments-by-id`  -  Put ContactDepartment
- `connectwise-manage-cli company put-contacts-relationships-by-id`  -  Put ContactRelationship
- `connectwise-manage-cli company put-contacts-types-by-id`  -  Put ContactType

**finance**  -  Manage finance

- `connectwise-manage-cli finance delete-agreements-by-id`  -  Delete Agreement
- `connectwise-manage-cli finance delete-agreements-types-by-id`  -  Delete AgreementType
- `connectwise-manage-cli finance delete-invoices-by-id`  -  Delete Invoice
- `connectwise-manage-cli finance get-agreements`  -  Get List of Agreement
- `connectwise-manage-cli finance get-agreements-by-id`  -  Get Agreement
- `connectwise-manage-cli finance get-agreements-by-parent-id-additions`  -  Get List of Addition
- `connectwise-manage-cli finance get-agreements-by-parent-id-adjustments`  -  Get List of Adjustment
- `connectwise-manage-cli finance get-agreements-by-parent-id-board-defaults`  -  Get List of BoardDefault
- `connectwise-manage-cli finance get-agreements-by-parent-id-configurations`  -  Get List of ConfigurationReference
- `connectwise-manage-cli finance get-agreements-by-parent-id-sites`  -  Get List of AgreementSite
- `connectwise-manage-cli finance get-agreements-by-parent-id-work-role-exclusions`  -  Get List of AgreementWorkRoleExclusion
- `connectwise-manage-cli finance get-agreements-by-parent-id-work-type-exclusions`  -  Get List of AgreementWorkTypeExclusion
- `connectwise-manage-cli finance get-agreements-by-parent-id-workroles`  -  Get List of AgreementWorkRole
- `connectwise-manage-cli finance get-agreements-by-parent-id-worktypes`  -  Get List of AgreementWorkType
- `connectwise-manage-cli finance get-agreements-count`  -  Get Count of Agreement
- `connectwise-manage-cli finance get-agreements-types`  -  Get List of AgreementType
- `connectwise-manage-cli finance get-agreements-types-by-id`  -  Get AgreementType
- `connectwise-manage-cli finance get-agreements-types-count`  -  Get Count of AgreementType
- `connectwise-manage-cli finance get-agreements-types-info`  -  Get List of AgreementTypeInfo
- `connectwise-manage-cli finance get-invoices`  -  Get List of Invoice
- `connectwise-manage-cli finance get-invoices-by-id`  -  Get Invoice
- `connectwise-manage-cli finance get-invoices-by-id-pdf`  -  Get Invoice
- `connectwise-manage-cli finance get-invoices-by-parent-id-commissions`  -  Get List of InvoiceCommissions
- `connectwise-manage-cli finance get-invoices-by-parent-id-gl-entries`  -  Get List of GLEntries
- `connectwise-manage-cli finance get-invoices-by-parent-id-payments`  -  Get List of Payment
- `connectwise-manage-cli finance get-invoices-by-parent-id-routings`  -  Get List of Invoice Routings
- `connectwise-manage-cli finance get-invoices-count`  -  Get Count of Invoice
- `connectwise-manage-cli finance patch-agreements-by-id`  -  Patch Agreement
- `connectwise-manage-cli finance patch-agreements-types-by-id`  -  Patch AgreementType
- `connectwise-manage-cli finance patch-invoices-by-id`  -  Patch Invoice
- `connectwise-manage-cli finance post-agreements`  -  Post Agreement
- `connectwise-manage-cli finance post-agreements-by-id-copy`  -  Post AgreementType
- `connectwise-manage-cli finance post-agreements-by-id-invoice`  -  Post AgreementInvoice
- `connectwise-manage-cli finance post-agreements-by-parent-id-additions`  -  Post Addition
- `connectwise-manage-cli finance post-agreements-by-parent-id-adjustments`  -  Post Adjustment
- `connectwise-manage-cli finance post-agreements-by-parent-id-board-defaults`  -  Post BoardDefault
- `connectwise-manage-cli finance post-agreements-by-parent-id-configurations`  -  Post ConfigurationReference
- `connectwise-manage-cli finance post-agreements-by-parent-id-copy`  -  Post CopyAgreementAction
- `connectwise-manage-cli finance post-agreements-by-parent-id-sites`  -  Post AgreementSite
- `connectwise-manage-cli finance post-agreements-by-parent-id-work-role-exclusions`  -  Post AgreementWorkRoleExclusion
- `connectwise-manage-cli finance post-agreements-by-parent-id-work-type-exclusions`  -  Post AgreementWorkTypeExclusion
- `connectwise-manage-cli finance post-agreements-by-parent-id-workroles`  -  Post AgreementWorkRole
- `connectwise-manage-cli finance post-agreements-by-parent-id-worktypes`  -  Post AgreementWorkType
- `connectwise-manage-cli finance post-agreements-types`  -  Post AgreementType
- `connectwise-manage-cli finance post-invoices`  -  Post Invoice
- `connectwise-manage-cli finance post-invoices-by-parent-id-payments`  -  Post Payment
- `connectwise-manage-cli finance post-invoices-by-parent-id-routings`  -  Post Invoice Routings
- `connectwise-manage-cli finance put-agreements-by-id`  -  Put Agreement
- `connectwise-manage-cli finance put-agreements-types-by-id`  -  Put AgreementType
- `connectwise-manage-cli finance put-invoices-by-id`  -  Put Invoice

**procurement**  -  Manage procurement

- `connectwise-manage-cli procurement delete-products-by-id`  -  Delete ProductItem
- `connectwise-manage-cli procurement delete-purchaseorders-by-id`  -  Delete PurchaseOrder
- `connectwise-manage-cli procurement delete-purchaseorders-by-parent-id-lineitems`  -  Delete PurchaseOrderLineItem
- `connectwise-manage-cli procurement get-products`  -  Get List of ProductItem
- `connectwise-manage-cli procurement get-products-by-id`  -  Get ProductItem
- `connectwise-manage-cli procurement get-products-by-parent-id-components`  -  Get List of ProductComponent
- `connectwise-manage-cli procurement get-products-by-parent-id-picking-shipping-details`  -  Get List of ProductPickingShippingDetail
- `connectwise-manage-cli procurement get-products-count`  -  Get Count of ProductItem
- `connectwise-manage-cli procurement get-purchaseorders`  -  Get List of PurchaseOrder
- `connectwise-manage-cli procurement get-purchaseorders-by-id`  -  Get PurchaseOrder
- `connectwise-manage-cli procurement get-purchaseorders-by-id-info`  -  Get PurchaseOrderInfo
- `connectwise-manage-cli procurement get-purchaseorders-by-parent-id-lineitems`  -  Get List of PurchaseOrderLineItem
- `connectwise-manage-cli procurement get-purchaseorders-count`  -  Get Count of PurchaseOrder
- `connectwise-manage-cli procurement get-purchaseorders-info`  -  Get List of PurchaseOrderInfo
- `connectwise-manage-cli procurement get-purchaseorders-info-count`  -  Get Count of PurchaseOrderInfo
- `connectwise-manage-cli procurement patch-products-by-id`  -  Patch ProductItem
- `connectwise-manage-cli procurement patch-purchaseorders-by-id`  -  Patch PurchaseOrder
- `connectwise-manage-cli procurement post-products`  -  Post ProductItem
- `connectwise-manage-cli procurement post-products-by-id-detach`  -  Post ProductDetach
- `connectwise-manage-cli procurement post-products-by-parent-id-components`  -  Post List of ProductComponent
- `connectwise-manage-cli procurement post-products-by-parent-id-picking-shipping-details`  -  Post List of ProductPickingShippingDetail
- `connectwise-manage-cli procurement post-purchaseorders`  -  Post PurchaseOrder
- `connectwise-manage-cli procurement post-purchaseorders-by-id-copy`  -  Post PurchaseOrderCopy
- `connectwise-manage-cli procurement post-purchaseorders-by-id-rebatch`  -  Post RebatchPurchaseOrder
- `connectwise-manage-cli procurement post-purchaseorders-by-id-unbatch`  -  Post UnbatchPurchaseOrder
- `connectwise-manage-cli procurement post-purchaseorders-by-parent-id-lineitems`  -  Post PurchaseOrderLineItem
- `connectwise-manage-cli procurement post-purchaseorders-by-parent-id-notes`  -  Post PurchaseOrderNote
- `connectwise-manage-cli procurement put-products-by-id`  -  Put ProductItem
- `connectwise-manage-cli procurement put-purchaseorders-by-id`  -  Put PurchaseOrder

**project**  -  Manage project

- `connectwise-manage-cli project delete-by-id`  -  Delete ApiProject
- `connectwise-manage-cli project delete-tickets-by-id`  -  Delete ProjectTicket
- `connectwise-manage-cli project get`  -  Get List of ApiProject
- `connectwise-manage-cli project get-by-id`  -  Get ApiProject
- `connectwise-manage-cli project get-by-id-workplan`  -  Get ProjectWorkplan
- `connectwise-manage-cli project get-by-parent-id-contacts`  -  Get List of ProjectContact
- `connectwise-manage-cli project get-by-parent-id-notes`  -  Get List of ProjectNote
- `connectwise-manage-cli project get-by-parent-id-phases`  -  Get List of ProjectPhase
- `connectwise-manage-cli project get-by-parent-id-team-members`  -  Get List of ProjectTeamMember
- `connectwise-manage-cli project get-count`  -  Get Count of ApiProject
- `connectwise-manage-cli project get-tickets`  -  Get List of ProjectTicket
- `connectwise-manage-cli project get-tickets-by-id`  -  Get ProjectTicket
- `connectwise-manage-cli project get-tickets-by-parent-id-activities`  -  Get List of ActivityReference Gets activities associated to the ticket Please use the /sales/activities?
- `connectwise-manage-cli project get-tickets-by-parent-id-all-notes`  -  Get List of ProjectTicketNote
- `connectwise-manage-cli project get-tickets-by-parent-id-configurations`  -  Get List of ConfigurationReference
- `connectwise-manage-cli project get-tickets-by-parent-id-documents`  -  Get List of DocumentReference Gets the documents associated to the ticket Please use the /system/documents?
- `connectwise-manage-cli project get-tickets-by-parent-id-notes`  -  Get List of TicketNote
- `connectwise-manage-cli project get-tickets-by-parent-id-products`  -  Get List of ProductReference Gets the products associated to the ticket Please use the /procurement/products?
- `connectwise-manage-cli project get-tickets-by-parent-id-scheduleentries`  -  Get List of ScheduleEntryReference Gets the schedule entries associated to the ticket Please use the /schedule/entries?
- `connectwise-manage-cli project get-tickets-by-parent-id-tasks`  -  Get List of TicketTask
- `connectwise-manage-cli project get-tickets-by-parent-id-timeentries`  -  Get List of TimeEntryReference Gets time entries associated to the ticket Please use the /time/entries?
- `connectwise-manage-cli project get-tickets-count`  -  Get Count of ProjectTicket
- `connectwise-manage-cli project patch-by-id`  -  Patch ApiProject
- `connectwise-manage-cli project patch-tickets-by-id`  -  Patch ProjectTicket
- `connectwise-manage-cli project post`  -  Post ApiProject
- `connectwise-manage-cli project post-by-parent-id-apply-templates`  -  Post ApplyTemplates
- `connectwise-manage-cli project post-by-parent-id-contacts`  -  Post ProjectContact
- `connectwise-manage-cli project post-by-parent-id-notes`  -  Post ProjectNote
- `connectwise-manage-cli project post-by-parent-id-phases`  -  Post ProjectPhase
- `connectwise-manage-cli project post-by-parent-id-team-members`  -  Post ProjectTeamMember
- `connectwise-manage-cli project post-tickets`  -  Post ProjectTicket
- `connectwise-manage-cli project post-tickets-by-parent-id-configurations`  -  Post ConfigurationReference
- `connectwise-manage-cli project post-tickets-by-parent-id-convert`  -  Post SuccessResponse
- `connectwise-manage-cli project post-tickets-by-parent-id-notes`  -  Post TicketNote
- `connectwise-manage-cli project post-tickets-by-parent-id-tasks`  -  Post TicketTask
- `connectwise-manage-cli project post-tickets-search`  -  Post List of ProjectTicket
- `connectwise-manage-cli project put-by-id`  -  Put ApiProject
- `connectwise-manage-cli project put-tickets-by-id`  -  Put ProjectTicket

**sales**  -  Manage sales

- `connectwise-manage-cli sales delete-activities-by-id`  -  Delete Activity
- `connectwise-manage-cli sales delete-activities-statuses-by-id`  -  Delete ActivityStatus
- `connectwise-manage-cli sales delete-activities-types-by-id`  -  Delete ActivityType
- `connectwise-manage-cli sales delete-opportunities-by-id`  -  Delete ConnectWise.Apis.v3_0.v2015_3.Sales.Opportunity.Opportunity
- `connectwise-manage-cli sales delete-opportunities-by-parent-id-forecast`  -  Delete Forecast
- `connectwise-manage-cli sales delete-opportunities-ratings-by-id`  -  Delete OpportunityRating
- `connectwise-manage-cli sales delete-opportunities-statuses-by-id`  -  Delete OpportunityStatus
- `connectwise-manage-cli sales delete-opportunities-types-by-id`  -  Delete OpportunityType
- `connectwise-manage-cli sales get-activities`  -  Get List of Activity
- `connectwise-manage-cli sales get-activities-by-id`  -  Get Activity
- `connectwise-manage-cli sales get-activities-count`  -  Get Count of Activity
- `connectwise-manage-cli sales get-activities-statuses`  -  Get List of ActivityStatus
- `connectwise-manage-cli sales get-activities-statuses-by-id`  -  Get ActivityStatus
- `connectwise-manage-cli sales get-activities-statuses-count`  -  Get Count of ActivityStatus
- `connectwise-manage-cli sales get-activities-statuses-info`  -  Get List of ActivityStatusInfos
- `connectwise-manage-cli sales get-activities-types`  -  Get List of ActivityType
- `connectwise-manage-cli sales get-activities-types-by-id`  -  Get ActivityType
- `connectwise-manage-cli sales get-activities-types-count`  -  Get Count of ActivityType
- `connectwise-manage-cli sales get-opportunities`  -  Get List of ConnectWise.Apis.v3_0.v2015_3.Sales.Opportunity.Opportunity
- `connectwise-manage-cli sales get-opportunities-by-id`  -  Get ConnectWise.Apis.v3_0.v2015_3.Sales.Opportunity.Opportunity
- `connectwise-manage-cli sales get-opportunities-by-parent-id-contacts`  -  Get List of OpportunityContact
- `connectwise-manage-cli sales get-opportunities-by-parent-id-forecast`  -  Get List of Forecast
- `connectwise-manage-cli sales get-opportunities-by-parent-id-notes`  -  Get List of OpportunityNote
- `connectwise-manage-cli sales get-opportunities-by-parent-id-team`  -  Get List of Team
- `connectwise-manage-cli sales get-opportunities-conversions-by-id`  -  Get Conversion
- `connectwise-manage-cli sales get-opportunities-count`  -  Get Count of ConnectWise.Apis.v3_0.v2015_3.Sales.Opportunity.Opportunity
- `connectwise-manage-cli sales get-opportunities-default`  -  Get ConnectWise.Apis.v3_0.v2015_3.Sales.Opportunity.Opportunity
- `connectwise-manage-cli sales get-opportunities-ratings`  -  Get List of OpportunityRating
- `connectwise-manage-cli sales get-opportunities-ratings-by-id`  -  Get OpportunityRating
- `connectwise-manage-cli sales get-opportunities-ratings-count`  -  Get Count of OpportunityRating
- `connectwise-manage-cli sales get-opportunities-ratings-info`  -  Get List of OpportunityRatingInfo
- `connectwise-manage-cli sales get-opportunities-statuses`  -  Get List of OpportunityStatus
- `connectwise-manage-cli sales get-opportunities-statuses-by-id`  -  Get OpportunityStatus
- `connectwise-manage-cli sales get-opportunities-statuses-count`  -  Get Count of OpportunityStatus
- `connectwise-manage-cli sales get-opportunities-statuses-info`  -  Get List of OpportunityStatusInfos
- `connectwise-manage-cli sales get-opportunities-types`  -  Get List of OpportunityType
- `connectwise-manage-cli sales get-opportunities-types-by-id`  -  Get OpportunityType
- `connectwise-manage-cli sales get-opportunities-types-count`  -  Get Count of OpportunityType
- `connectwise-manage-cli sales get-opportunities-types-info`  -  Get List of OpportunityTypeInfos
- `connectwise-manage-cli sales patch-activities-by-id`  -  Patch Activity
- `connectwise-manage-cli sales patch-activities-statuses-by-id`  -  Patch ActivityStatus
- `connectwise-manage-cli sales patch-activities-types-by-id`  -  Patch ActivityType
- `connectwise-manage-cli sales patch-opportunities-by-id`  -  Patch ConnectWise.Apis.v3_0.v2015_3.Sales.Opportunity.Opportunity
- `connectwise-manage-cli sales patch-opportunities-by-parent-id-forecast`  -  Patch Forecast
- `connectwise-manage-cli sales patch-opportunities-ratings-by-id`  -  Patch OpportunityRating
- `connectwise-manage-cli sales patch-opportunities-statuses-by-id`  -  Patch OpportunityStatus
- `connectwise-manage-cli sales patch-opportunities-types-by-id`  -  Patch OpportunityType
- `connectwise-manage-cli sales post-activities`  -  Post Activity
- `connectwise-manage-cli sales post-activities-statuses`  -  Post ActivityStatus
- `connectwise-manage-cli sales post-activities-types`  -  Post ActivityType
- `connectwise-manage-cli sales post-opportunities`  -  Post ConnectWise.Apis.v3_0.v2015_3.Sales.Opportunity.Opportunity
- `connectwise-manage-cli sales post-opportunities-by-id-convert-to-agreement`  -  Post ApiAgreement
- `connectwise-manage-cli sales post-opportunities-by-id-convert-to-order`  -  Post ApiSalesOrder
- `connectwise-manage-cli sales post-opportunities-by-id-convert-to-project`  -  Post ApiProject
- `connectwise-manage-cli sales post-opportunities-by-id-convert-to-service-ticket`  -  Post ApiTicket
- `connectwise-manage-cli sales post-opportunities-by-parent-id-contacts`  -  Post OpportunityContact
- `connectwise-manage-cli sales post-opportunities-by-parent-id-forecast`  -  Post Forecast
- `connectwise-manage-cli sales post-opportunities-by-parent-id-notes`  -  Post OpportunityNote
- `connectwise-manage-cli sales post-opportunities-by-parent-id-team`  -  Post Team
- `connectwise-manage-cli sales post-opportunities-ratings`  -  Post OpportunityRating
- `connectwise-manage-cli sales post-opportunities-statuses`  -  Post OpportunityStatus
- `connectwise-manage-cli sales post-opportunities-types`  -  Post OpportunityType
- `connectwise-manage-cli sales put-activities-by-id`  -  Put Activity
- `connectwise-manage-cli sales put-activities-statuses-by-id`  -  Put ActivityStatus
- `connectwise-manage-cli sales put-activities-types-by-id`  -  Put ActivityType
- `connectwise-manage-cli sales put-opportunities-by-id`  -  Put ConnectWise.Apis.v3_0.v2015_3.Sales.Opportunity.Opportunity
- `connectwise-manage-cli sales put-opportunities-by-parent-id-forecast`  -  Put Forecast
- `connectwise-manage-cli sales put-opportunities-ratings-by-id`  -  Put OpportunityRating
- `connectwise-manage-cli sales put-opportunities-statuses-by-id`  -  Put OpportunityStatus
- `connectwise-manage-cli sales put-opportunities-types-by-id`  -  Put OpportunityType

**service**  -  Manage service

- `connectwise-manage-cli service delete-boards-by-id`  -  Delete Board
- `connectwise-manage-cli service delete-priorities-by-id`  -  Delete Priority
- `connectwise-manage-cli service delete-sources-by-id`  -  Delete Source
- `connectwise-manage-cli service delete-tickets-by-id`  -  Delete Ticket
- `connectwise-manage-cli service delete-tickets-by-parent-id-configurations-by-id`  -  Delete ConfigurationReference
- `connectwise-manage-cli service delete-tickets-by-parent-id-notes-by-id`  -  Delete ServiceNote
- `connectwise-manage-cli service delete-tickets-by-parent-id-tasks-by-id`  -  Delete Task
- `connectwise-manage-cli service delete-tickets-changelogs`  -  Delete Ticket Change Logs
- `connectwise-manage-cli service get-boards`  -  Get List of Board
- `connectwise-manage-cli service get-boards-by-id`  -  Get Board
- `connectwise-manage-cli service get-boards-by-id-usages`  -  Get List of Usage Count
- `connectwise-manage-cli service get-boards-by-parent-id-statuses`  -  Get List of BoardStatus
- `connectwise-manage-cli service get-boards-by-parent-id-type-sub-type-item-associations`  -  Get List of BoardTypeSubTypeItemAssociation
- `connectwise-manage-cli service get-boards-count`  -  Get Count of Board
- `connectwise-manage-cli service get-priorities`  -  Get List of Priority
- `connectwise-manage-cli service get-priorities-by-id`  -  Get Priority
- `connectwise-manage-cli service get-priorities-by-id-image`  -  Get Priority
- `connectwise-manage-cli service get-priorities-by-id-usages`  -  Get List of Usage Count
- `connectwise-manage-cli service get-priorities-count`  -  Get Count of Priority
- `connectwise-manage-cli service get-sources`  -  Get List of Source
- `connectwise-manage-cli service get-sources-by-id`  -  Get Source
- `connectwise-manage-cli service get-sources-by-id-info`  -  Get SourceInfos
- `connectwise-manage-cli service get-sources-by-id-usages`  -  Get List of Usage Count
- `connectwise-manage-cli service get-sources-count`  -  Get Count of Source
- `connectwise-manage-cli service get-sources-info`  -  Get List of SourceInfos
- `connectwise-manage-cli service get-sources-info-count`  -  Get Count of SourceInfo
- `connectwise-manage-cli service get-tickets`  -  Get List of ConnectWise.Apis.v3_0.v2015_3.Service.Ticket.Ticket
- `connectwise-manage-cli service get-tickets-by-id`  -  Get Ticket
- `connectwise-manage-cli service get-tickets-by-id-info`  -  Get TicketInfos
- `connectwise-manage-cli service get-tickets-by-parent-id-activities`  -  Get List of ActivityReference Gets activities associated to the ticket Please use the /sales/activities?
- `connectwise-manage-cli service get-tickets-by-parent-id-activities-count`  -  Get Count of ActivityReference Gets count of activities associated to the ticket Please use the /sales/activities/count?
- `connectwise-manage-cli service get-tickets-by-parent-id-all-notes`  -  Get List of ServiceTicketNote
- `connectwise-manage-cli service get-tickets-by-parent-id-configurations`  -  Get List of ConfigurationReference
- `connectwise-manage-cli service get-tickets-by-parent-id-configurations-by-id`  -  Get ConfigurationReference
- `connectwise-manage-cli service get-tickets-by-parent-id-configurations-count`  -  Get Count of ConfigurationReference
- `connectwise-manage-cli service get-tickets-by-parent-id-documents`  -  Get List of DocumentReference Gets the documents associated to the ticket Please use the /system/documents?
- `connectwise-manage-cli service get-tickets-by-parent-id-documents-count`  -  Get Count of DocumentReference
- `connectwise-manage-cli service get-tickets-by-parent-id-notes`  -  Get List of ServiceNote
- `connectwise-manage-cli service get-tickets-by-parent-id-notes-by-id`  -  Get ServiceNote
- `connectwise-manage-cli service get-tickets-by-parent-id-notes-count`  -  Get Count of ServiceNote
- `connectwise-manage-cli service get-tickets-by-parent-id-products`  -  Get List of ProductReference Gets the products associated to the ticket Please use the /procurement/products?
- `connectwise-manage-cli service get-tickets-by-parent-id-products-count`  -  Get Count of ProductReference Gets the products associated to the ticket Please use the /procurement/products/count?
- `connectwise-manage-cli service get-tickets-by-parent-id-scheduleentries`  -  Get List of ScheduleEntryReference Gets the schedule entries associated to the ticket Please use the /schedule/entries?
- `connectwise-manage-cli service get-tickets-by-parent-id-scheduleentries-count`  -  Get Count of ScheduleEntryReference Gets the schedule entries count associated to the ticket Please use the
- `connectwise-manage-cli service get-tickets-by-parent-id-tasks`  -  Get List of Task
- `connectwise-manage-cli service get-tickets-by-parent-id-tasks-by-id`  -  Get Task
- `connectwise-manage-cli service get-tickets-by-parent-id-tasks-count`  -  Get Count of Task
- `connectwise-manage-cli service get-tickets-by-parent-id-timeentries`  -  Get List of TimeEntryReference Gets time entries associated to the ticket Please use the /time/entries?
- `connectwise-manage-cli service get-tickets-by-parent-id-timeentries-count`  -  Get Count of TimeEntryReference Gets time entries count associated to the ticket Please use the /time/entries/count?
- `connectwise-manage-cli service get-tickets-calculate-sla`  -  Get List of ConnectWise.Apis.v3_0.v2015_3.Service.Ticket.Ticket with SLA calculated
- `connectwise-manage-cli service get-tickets-changelogs`  -  Get List of Ticket Change Log
- `connectwise-manage-cli service get-tickets-count`  -  Get Count of ConnectWise.Apis.v3_0.v2015_3.Service.Ticket.Ticket
- `connectwise-manage-cli service get-tickets-info`  -  Get List of TicketInfos
- `connectwise-manage-cli service get-tickets-info-count`  -  Get Count of TicketInfo
- `connectwise-manage-cli service patch-boards-by-id`  -  Patch Board
- `connectwise-manage-cli service patch-priorities-by-id`  -  Patch Priority
- `connectwise-manage-cli service patch-sources-by-id`  -  Patch Source
- `connectwise-manage-cli service patch-tickets-by-id`  -  Patch Ticket
- `connectwise-manage-cli service patch-tickets-by-parent-id-notes-by-id`  -  Patch ServiceNote
- `connectwise-manage-cli service patch-tickets-by-parent-id-tasks-by-id`  -  Patch Task
- `connectwise-manage-cli service post-boards`  -  Post Board
- `connectwise-manage-cli service post-boards-by-parent-id-statuses`  -  Post BoardStatus
- `connectwise-manage-cli service post-boards-copy`  -  Post Board
- `connectwise-manage-cli service post-priorities`  -  Post Priority
- `connectwise-manage-cli service post-sources`  -  Post Source
- `connectwise-manage-cli service post-tickets`  -  Post Ticket
- `connectwise-manage-cli service post-tickets-by-id-copy`  -  Post TicketCopy
- `connectwise-manage-cli service post-tickets-by-parent-id-attach-children`  -  Post SuccessResponse
- `connectwise-manage-cli service post-tickets-by-parent-id-configurations`  -  Post ConfigurationReference
- `connectwise-manage-cli service post-tickets-by-parent-id-convert`  -  Post SuccessResponse
- `connectwise-manage-cli service post-tickets-by-parent-id-merge`  -  Post SuccessResponse
- `connectwise-manage-cli service post-tickets-by-parent-id-notes`  -  Post ServiceNote
- `connectwise-manage-cli service post-tickets-by-parent-id-tasks`  -  Post Task
- `connectwise-manage-cli service post-tickets-search`  -  Post List of Ticket
- `connectwise-manage-cli service put-boards-by-id`  -  Put Board
- `connectwise-manage-cli service put-priorities-by-id`  -  Put Priority
- `connectwise-manage-cli service put-sources-by-id`  -  Put Source
- `connectwise-manage-cli service put-tickets-by-id`  -  Put Ticket
- `connectwise-manage-cli service put-tickets-by-parent-id-notes-by-id`  -  Put ServiceNote
- `connectwise-manage-cli service put-tickets-by-parent-id-tasks-by-id`  -  Put Task

**system**  -  Manage system

- `connectwise-manage-cli system delete-members-by-id-unused-time-sheets`  -  Delete Member
- `connectwise-manage-cli system delete-members-types-by-id`  -  Delete MemberType
- `connectwise-manage-cli system get-info`  -  Get Info
- `connectwise-manage-cli system get-info-departmentlocations`  -  Get List of DepartmentLocationInfo
- `connectwise-manage-cli system get-info-departmentlocations-by-id`  -  Get DepartmentLocationInfo
- `connectwise-manage-cli system get-info-departmentlocations-count`  -  Get Count of DepartmentLocationInfo
- `connectwise-manage-cli system get-info-departments`  -  Get List of DepartmentInfo
- `connectwise-manage-cli system get-info-departments-by-id`  -  Get DepartmentInfo
- `connectwise-manage-cli system get-info-departments-count`  -  Get Count of DepartmentInfo
- `connectwise-manage-cli system get-info-links`  -  Get List of LinkInfo
- `connectwise-manage-cli system get-info-links-by-id`  -  Get LinkInfo
- `connectwise-manage-cli system get-info-links-count`  -  Get Count of LinkInfo
- `connectwise-manage-cli system get-info-locales`  -  Get List of LocaleInfo
- `connectwise-manage-cli system get-info-locales-by-id`  -  Get LocaleInfo
- `connectwise-manage-cli system get-info-locales-count`  -  Get Count of LocaleInfo
- `connectwise-manage-cli system get-info-locations`  -  Get List of LocationInfo
- `connectwise-manage-cli system get-info-locations-by-id`  -  Get LocationInfo
- `connectwise-manage-cli system get-info-locations-count`  -  Get Count of LocationInfo
- `connectwise-manage-cli system get-info-members`  -  Get List of MemberInfo
- `connectwise-manage-cli system get-info-members-by-id`  -  Get MemberInfo
- `connectwise-manage-cli system get-info-members-count`  -  Get Count of MemberInfo
- `connectwise-manage-cli system get-info-membersmember-identifierregextypes`  -  Get MemberInfo
- `connectwise-manage-cli system get-info-personas`  -  Get List of PersonasInfo
- `connectwise-manage-cli system get-info-personas-by-id`  -  Get PersonasInfo
- `connectwise-manage-cli system get-info-personas-count`  -  Get Count of PersonasInfo
- `connectwise-manage-cli system get-info-standard-notes`  -  Get List of StandardNoteInfo
- `connectwise-manage-cli system get-info-standard-notes-by-id`  -  Get StandardNoteInfo
- `connectwise-manage-cli system get-info-standard-notes-count`  -  Get Count of StandardNoteInfo
- `connectwise-manage-cli system get-members`  -  Get List of Member
- `connectwise-manage-cli system get-members-by-id`  -  Get Member
- `connectwise-manage-cli system get-members-by-id-image`  -  Get
- `connectwise-manage-cli system get-members-by-id-usages`  -  Get List of Usage Count
- `connectwise-manage-cli system get-members-by-parent-id-certifications`  -  Get List of MemberCertification
- `connectwise-manage-cli system get-members-by-parent-id-managed-device-accounts`  -  Get List of ManagedDeviceAccount
- `connectwise-manage-cli system get-members-by-parent-id-mycertifications`  -  Get List of MemberCertification
- `connectwise-manage-cli system get-members-by-parent-id-notification-settings`  -  Get List of MemberNotificationSetting
- `connectwise-manage-cli system get-members-by-parent-id-personas`  -  Get List of MemberPersona
- `connectwise-manage-cli system get-members-calendarsync`  -  Get List of Member to be use for calendar sync subscriptions
- `connectwise-manage-cli system get-members-count`  -  Get Count of Usage
- `connectwise-manage-cli system get-members-types`  -  Get List of MemberType
- `connectwise-manage-cli system get-members-types-by-id`  -  Get MemberType
- `connectwise-manage-cli system get-members-types-count`  -  Get Count of MemberType
- `connectwise-manage-cli system get-members-types-info`  -  Get List of MemberType
- `connectwise-manage-cli system get-members-with-sso`  -  Get List of Member
- `connectwise-manage-cli system patch-members-by-id`  -  Patch Member
- `connectwise-manage-cli system patch-members-types-by-id`  -  Patch MemberType
- `connectwise-manage-cli system post-members`  -  Post Member
- `connectwise-manage-cli system post-members-by-id-deactivate`  -  Post MemberDeactivation
- `connectwise-manage-cli system post-members-by-id-link-sso-user`  -  Post SuccessResponse
- `connectwise-manage-cli system post-members-by-id-submit`  -  Post SuccessResponse
- `connectwise-manage-cli system post-members-by-id-unlink-sso-user`  -  Post SuccessResponse
- `connectwise-manage-cli system post-members-by-member-identifier-tokens`  -  Post Token
- `connectwise-manage-cli system post-members-by-parent-id-certifications`  -  Post MemberCertification
- `connectwise-manage-cli system post-members-by-parent-id-mycertifications`  -  Post MemberCertification
- `connectwise-manage-cli system post-members-by-parent-id-notification-settings`  -  Post MemberNotificationSetting
- `connectwise-manage-cli system post-members-by-parent-id-personas`  -  Post MemberPersona
- `connectwise-manage-cli system post-members-by-ssoid-deactivate-iam-member`  -  Delete Member Via IAM
- `connectwise-manage-cli system post-members-types`  -  Post MemberType
- `connectwise-manage-cli system put-members-by-id`  -  Put Member
- `connectwise-manage-cli system put-members-types-by-id`  -  Put MemberType

**time**  -  Manage time

- `connectwise-manage-cli time delete-entries-by-id`  -  Delete TimeEntry
- `connectwise-manage-cli time delete-sheets-by-id`  -  Delete TimeSheet
- `connectwise-manage-cli time get-entries`  -  Get List of TimeEntry
- `connectwise-manage-cli time get-entries-by-id`  -  Get TimeEntry
- `connectwise-manage-cli time get-entries-by-parent-id-audits`  -  Get List of TimeEntryAudit
- `connectwise-manage-cli time get-entries-count`  -  Get Count of TimeEntry
- `connectwise-manage-cli time get-sheets`  -  Get List of TimeSheet
- `connectwise-manage-cli time get-sheets-by-id`  -  Get TimeSheet
- `connectwise-manage-cli time get-sheets-by-parent-id-audits`  -  Get List of TimeSheetAudit
- `connectwise-manage-cli time get-sheets-count`  -  Get Count of TimeSheet
- `connectwise-manage-cli time patch-entries-by-id`  -  Patch TimeEntry
- `connectwise-manage-cli time post-entries`  -  Post TimeEntry
- `connectwise-manage-cli time post-entries-defaults`  -  Post TimeEntry
- `connectwise-manage-cli time post-sheets-by-id-approve`  -  Post SuccessResponse
- `connectwise-manage-cli time post-sheets-by-id-reject`  -  Post SuccessResponse
- `connectwise-manage-cli time post-sheets-by-id-reverse`  -  Post SuccessResponse
- `connectwise-manage-cli time post-sheets-by-id-submit`  -  Post SuccessResponse
- `connectwise-manage-cli time put-entries-by-id`  -  Put TimeEntry


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
connectwise-manage-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Triage new tickets, agent-friendly

```bash
connectwise-manage-cli service get-tickets --conditions 'status/name="New"' --agent --select id,summary,board.name,company.identifier,owner.identifier
```

Narrows the deeply-nested ticket payload to the five fields an agent actually needs, so it doesn't burn context parsing the full record.

### Find unbilled work before the billing cutoff

```bash
connectwise-manage-cli unbilled --since 7d --agent
```

Lists tickets touched this week with zero or under-threshold time logged  -  the join the PSA won't give you.

### Account 360 before a QBR

```bash
connectwise-manage-cli account "Acme Corp" --agent
```

One card with contacts, active agreements, deployed configurations, and open-ticket count instead of five web screens.

### Build a safe conditions filter

```bash
connectwise-manage-cli condition build --field board/id --op in --value 2,3 --field status/name --op = --value New
```

Emits a validated conditions string (correct quoting, AND join) you can paste into any list command.

## Auth Setup

ConnectWise Manage uses HTTP Basic auth with a twist: the username is the composite `companyId+publicKey` and the password is the `privateKey`, plus a `clientId` GUID header is required on every call (registered at developer.connectwise.com). Set CW_COMPANY_ID, CW_PUBLIC_KEY, CW_PRIVATE_KEY, and CW_CLIENT_ID; set CW_SITE to your region host (api-na, api-eu, api-au) or your on-prem host. Run `doctor` to validate all four credentials and reachability before anything else.

Run `connectwise-manage-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  connectwise-manage-cli project get --agent --select id,name,status
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success, and `--ignore-missing` only when a missing delete target should count as success

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
connectwise-manage-cli feedback "the --since flag is inclusive but docs say exclusive"
connectwise-manage-cli feedback --stdin < notes.txt
connectwise-manage-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/connectwise-manage-cli/feedback.jsonl`. They are never POSTed unless `CONNECTWISE_MANAGE_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `CONNECTWISE_MANAGE_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
connectwise-manage-cli profile save briefing --json
connectwise-manage-cli --profile briefing project get
connectwise-manage-cli profile list --json
connectwise-manage-cli profile show briefing
connectwise-manage-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `connectwise-manage-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/project-management/connectwise-manage/cmd/connectwise-manage-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add connectwise-manage-mcp -- connectwise-manage-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which connectwise-manage-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   connectwise-manage-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `connectwise-manage-cli <command> --help`.
