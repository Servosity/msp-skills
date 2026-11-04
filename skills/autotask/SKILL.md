---
name: autotask
description: "Every Autotask entity at the command line, plus a local SQLite mirror that answers ticket-aging, workload, unbilled-time, and account-360 questions no other Autotask tool can. Trigger phrases: `list autotask tickets`, `autotask company 360`, `who is overloaded in autotask`, `unbilled time in autotask`, `autotask ticket aging`, `use autotask`, `run autotask-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Autotask"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - autotask-cli
    install:
      - kind: script
        bins: [autotask-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/autotask/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/autotask/install.ps1
---

# Autotask PSA - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `autotask-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/autotask/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/autotask/install.ps1 | iex
   ```
3. Verify: `autotask-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `autotask-cli` and `autotask-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Autotask PSA CLI syncs your PSA into a local database and hides Autotask's non-standard query/paging mechanics behind clean list/search/get commands. Beyond CRUD, it ships service-desk intelligence  -  ticket-aging, workload, sla-breaches, triage  -  and money views like unbilled and contract-burn, all offline, scriptable, and agent-native.

## When to Use This CLI

Use this CLI when an agent or technician needs to read, search, or report on an Autotask PSA tenant from the terminal  -  service-desk triage, time/billing reconciliation, contract burn, account 360, or any analytics that requires joining across tickets, companies, contracts, and time. Prefer it over raw API calls whenever the answer requires aggregation across many records.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI to administer Autotask configuration (workflows, security levels, UI settings)  -  it covers the REST entity surface only.
- Do not use the novel analytics commands (triage, unbilled, company-360, reconcile, retainer, ...) without running sync first  -  they read the local SQLite mirror, not the live API.
- Do not use this CLI for bulk destructive operations across entities; deletes are per-record and require explicit ids.
- For one-off raw API exploration with full control of filters, prefer the per-entity query commands over search  -  search hits the local FTS index.

## Unique Capabilities

### Service-desk intelligence
- **`ticket-aging`**  -  Bucket open tickets by age so you see how stale the service desk is at a glance.

  _Reach for this when an agent needs service-desk health, not a raw ticket dump._

  ```bash
  autotask-cli ticket-aging --by queue --agent
  ```
- **`workload`**  -  See which technicians are overloaded by open ticket and task hours before you assign more.

  _Use before dispatch/assignment decisions._

  ```bash
  autotask-cli workload --agent
  ```
- **`sla-breaches`**  -  List open tickets past their due date or overdue for first response.

  _Use to catch SLA risk before the customer does._

  ```bash
  autotask-cli sla-breaches --agent
  ```
- **`stale`**  -  Find tickets or projects with no activity in N days.

  _Use to clean up neglected work._

  ```bash
  autotask-cli stale --days 14 --entity tickets --agent
  ```
- **`since`**  -  See what changed across tickets in the last N hours or days.

  _Use for shift handoff and 'what did I miss'._

  ```bash
  autotask-cli since 24h --agent
  ```
- **`triage`**  -  Rank open unassigned tickets by priority and age into a workable queue.

  _Use as the dispatcher's first command of the day._

  ```bash
  autotask-cli triage --agent
  ```

### Money and contracts
- **`unbilled`**  -  Surface billable time entries not yet approved and posted for billing  -  money on the table.

  _Use for billing reconciliation and revenue recovery._

  ```bash
  autotask-cli unbilled --company 1234 --agent
  ```
- **`contract-burn`**  -  Show how much of each contract's hours/blocks are consumed versus purchased.

  _Use to spot contracts about to run dry before they do._

  ```bash
  autotask-cli contract-burn --agent
  ```
- **`reconcile`**  -  One month-end table: billable time not yet approved and posted, contract blocks consumed vs purchased, and the money left on the table.

  _Reach for this when an agent needs the full billing-close picture, not just unbilled hours._

  ```bash
  autotask-cli reconcile --company 1234 --agent
  ```
- **`retainer`**  -  Block-hour contracts ranked by percent consumed, with projected run-out dates from recent burn rate.

  _Use to flag retainers about to run dry before the customer disputes an overage._

  ```bash
  autotask-cli retainer --threshold 80 --agent
  ```

### Account 360
- **`company-360`**  -  One view of a company's tickets, contacts, contracts, config items, and opportunities.

  _Use for account reviews and escalations._

  ```bash
  autotask-cli company-360 1234 --agent
  ```
- **`project-health`**  -  Flag projects with overdue tasks or past end dates.

  _Use for PM standups and status reporting._

  ```bash
  autotask-cli project-health --agent
  ```
- **`account-brief`**  -  What changed on one account since a point in time  -  new tickets, contract burn movement, opportunities, stale items.

  _Use before an account call to see what moved since the last one; company-360 is the full current snapshot._

  ```bash
  autotask-cli account-brief 1234 --since 7d --agent
  ```

### Data hygiene and metadata
- **`data-gaps`**  -  Find tickets with no contract, time entries with no ticket, contacts and config items with no company.

  _Run before billing or reporting to catch the broken links that silently corrupt both._

  ```bash
  autotask-cli data-gaps --entity tickets --agent
  ```
- **`picklist`**  -  Print the label-to-ID map for any picklist field (status, priority, queue) from cached field metadata.

  _Use to translate Autotask integer IDs before building filters; use the entity's query-field-definitions command to list all fields._

  ```bash
  autotask-cli picklist TimeEntries timeEntryType --agent
  ```

## Command Reference

**appointments**  -  Manage appointments

- `autotask-cli appointments create-entity`  -  Create entity
- `autotask-cli appointments delete-entity`  -  Delete entity
- `autotask-cli appointments patch-entity`  -  Patch entity
- `autotask-cli appointments query`  -  Query
- `autotask-cli appointments query-count`  -  Query count
- `autotask-cli appointments query-entity-information`  -  Query entity information
- `autotask-cli appointments query-field-definitions`  -  Query field definitions
- `autotask-cli appointments query-item`  -  Query item
- `autotask-cli appointments query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli appointments update-entity`  -  Update entity
- `autotask-cli appointments url-parameter-query`  -  Url parameter query
- `autotask-cli appointments url-parameter-query-count`  -  Url parameter query count

**autotask-psa-version**  -  Manage autotask psa version

- `autotask-cli autotask-psa-version`  -  Autotask api integration query information

**billing-codes**  -  Manage billing codes

- `autotask-cli billing-codes query`  -  Query
- `autotask-cli billing-codes query-count`  -  Query count
- `autotask-cli billing-codes query-entity-information`  -  Query entity information
- `autotask-cli billing-codes query-field-definitions`  -  Query field definitions
- `autotask-cli billing-codes query-item`  -  Query item
- `autotask-cli billing-codes query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli billing-codes url-parameter-query`  -  Url parameter query
- `autotask-cli billing-codes url-parameter-query-count`  -  Url parameter query count

**companies**  -  Manage companies

- `autotask-cli companies create-entity`  -  Create entity
- `autotask-cli companies patch-entity`  -  Patch entity
- `autotask-cli companies query`  -  Query
- `autotask-cli companies query-count`  -  Query count
- `autotask-cli companies query-entity-information`  -  Query entity information
- `autotask-cli companies query-field-definitions`  -  Query field definitions
- `autotask-cli companies query-item`  -  Query item
- `autotask-cli companies query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli companies update-entity`  -  Update entity
- `autotask-cli companies url-parameter-query`  -  Url parameter query
- `autotask-cli companies url-parameter-query-count`  -  Url parameter query count

**company-notes**  -  Manage company notes

- `autotask-cli company-notes query`  -  Query
- `autotask-cli company-notes query-count`  -  Query count
- `autotask-cli company-notes query-entity-information`  -  Query entity information
- `autotask-cli company-notes query-field-definitions`  -  Query field definitions
- `autotask-cli company-notes query-item`  -  Query item
- `autotask-cli company-notes query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli company-notes url-parameter-query`  -  Url parameter query
- `autotask-cli company-notes url-parameter-query-count`  -  Url parameter query count

**configuration-items**  -  Manage configuration items

- `autotask-cli configuration-items create-entity`  -  Create entity
- `autotask-cli configuration-items patch-entity`  -  Patch entity
- `autotask-cli configuration-items query`  -  Query
- `autotask-cli configuration-items query-count`  -  Query count
- `autotask-cli configuration-items query-entity-information`  -  Query entity information
- `autotask-cli configuration-items query-field-definitions`  -  Query field definitions
- `autotask-cli configuration-items query-item`  -  Query item
- `autotask-cli configuration-items query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli configuration-items update-entity`  -  Update entity
- `autotask-cli configuration-items url-parameter-query`  -  Url parameter query
- `autotask-cli configuration-items url-parameter-query-count`  -  Url parameter query count

**contact-groups**  -  Manage contact groups

- `autotask-cli contact-groups create-entity`  -  Create entity
- `autotask-cli contact-groups delete-entity`  -  Delete entity
- `autotask-cli contact-groups patch-entity`  -  Patch entity
- `autotask-cli contact-groups query`  -  Query
- `autotask-cli contact-groups query-count`  -  Query count
- `autotask-cli contact-groups query-entity-information`  -  Query entity information
- `autotask-cli contact-groups query-field-definitions`  -  Query field definitions
- `autotask-cli contact-groups query-item`  -  Query item
- `autotask-cli contact-groups query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli contact-groups update-entity`  -  Update entity
- `autotask-cli contact-groups url-parameter-query`  -  Url parameter query
- `autotask-cli contact-groups url-parameter-query-count`  -  Url parameter query count

**contacts**  -  Manage contacts

- `autotask-cli contacts query`  -  Query
- `autotask-cli contacts query-count`  -  Query count
- `autotask-cli contacts query-entity-information`  -  Query entity information
- `autotask-cli contacts query-field-definitions`  -  Query field definitions
- `autotask-cli contacts query-item`  -  Query item
- `autotask-cli contacts query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli contacts url-parameter-query`  -  Url parameter query
- `autotask-cli contacts url-parameter-query-count`  -  Url parameter query count

**contract-blocks**  -  Manage contract blocks

- `autotask-cli contract-blocks query`  -  Query
- `autotask-cli contract-blocks query-count`  -  Query count
- `autotask-cli contract-blocks query-entity-information`  -  Query entity information
- `autotask-cli contract-blocks query-field-definitions`  -  Query field definitions
- `autotask-cli contract-blocks query-item`  -  Query item
- `autotask-cli contract-blocks query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli contract-blocks url-parameter-query`  -  Url parameter query
- `autotask-cli contract-blocks url-parameter-query-count`  -  Url parameter query count

**contract-notes**  -  Manage contract notes

- `autotask-cli contract-notes query`  -  Query
- `autotask-cli contract-notes query-count`  -  Query count
- `autotask-cli contract-notes query-entity-information`  -  Query entity information
- `autotask-cli contract-notes query-field-definitions`  -  Query field definitions
- `autotask-cli contract-notes query-item`  -  Query item
- `autotask-cli contract-notes query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli contract-notes url-parameter-query`  -  Url parameter query
- `autotask-cli contract-notes url-parameter-query-count`  -  Url parameter query count

**contract-rates**  -  Manage contract rates

- `autotask-cli contract-rates query`  -  Query
- `autotask-cli contract-rates query-count`  -  Query count
- `autotask-cli contract-rates query-entity-information`  -  Query entity information
- `autotask-cli contract-rates query-field-definitions`  -  Query field definitions
- `autotask-cli contract-rates query-item`  -  Query item
- `autotask-cli contract-rates query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli contract-rates url-parameter-query`  -  Url parameter query
- `autotask-cli contract-rates url-parameter-query-count`  -  Url parameter query count

**contract-service-units**  -  Manage contract service units

- `autotask-cli contract-service-units query`  -  Query
- `autotask-cli contract-service-units query-count`  -  Query count
- `autotask-cli contract-service-units query-entity-information`  -  Query entity information
- `autotask-cli contract-service-units query-field-definitions`  -  Query field definitions
- `autotask-cli contract-service-units query-item`  -  Query item
- `autotask-cli contract-service-units query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli contract-service-units url-parameter-query`  -  Url parameter query
- `autotask-cli contract-service-units url-parameter-query-count`  -  Url parameter query count

**contract-services**  -  Manage contract services

- `autotask-cli contract-services query`  -  Query
- `autotask-cli contract-services query-count`  -  Query count
- `autotask-cli contract-services query-entity-information`  -  Query entity information
- `autotask-cli contract-services query-field-definitions`  -  Query field definitions
- `autotask-cli contract-services query-item`  -  Query item
- `autotask-cli contract-services query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli contract-services url-parameter-query`  -  Url parameter query
- `autotask-cli contract-services url-parameter-query-count`  -  Url parameter query count

**contracts**  -  Manage contracts

- `autotask-cli contracts create-entity`  -  Create entity
- `autotask-cli contracts patch-entity`  -  Patch entity
- `autotask-cli contracts query`  -  Query
- `autotask-cli contracts query-count`  -  Query count
- `autotask-cli contracts query-entity-information`  -  Query entity information
- `autotask-cli contracts query-field-definitions`  -  Query field definitions
- `autotask-cli contracts query-item`  -  Query item
- `autotask-cli contracts query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli contracts update-entity`  -  Update entity
- `autotask-cli contracts url-parameter-query`  -  Url parameter query
- `autotask-cli contracts url-parameter-query-count`  -  Url parameter query count

**countries**  -  Manage countries

- `autotask-cli countries patch-entity`  -  Patch entity
- `autotask-cli countries query`  -  Query
- `autotask-cli countries query-count`  -  Query count
- `autotask-cli countries query-entity-information`  -  Query entity information
- `autotask-cli countries query-field-definitions`  -  Query field definitions
- `autotask-cli countries query-item`  -  Query item
- `autotask-cli countries query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli countries update-entity`  -  Update entity
- `autotask-cli countries url-parameter-query`  -  Url parameter query
- `autotask-cli countries url-parameter-query-count`  -  Url parameter query count

**currencies**  -  Manage currencies

- `autotask-cli currencies patch-entity`  -  Patch entity
- `autotask-cli currencies query`  -  Query
- `autotask-cli currencies query-count`  -  Query count
- `autotask-cli currencies query-entity-information`  -  Query entity information
- `autotask-cli currencies query-field-definitions`  -  Query field definitions
- `autotask-cli currencies query-item`  -  Query item
- `autotask-cli currencies query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli currencies update-entity`  -  Update entity
- `autotask-cli currencies url-parameter-query`  -  Url parameter query
- `autotask-cli currencies url-parameter-query-count`  -  Url parameter query count

**departments**  -  Manage departments

- `autotask-cli departments create-entity`  -  Create entity
- `autotask-cli departments patch-entity`  -  Patch entity
- `autotask-cli departments query`  -  Query
- `autotask-cli departments query-count`  -  Query count
- `autotask-cli departments query-entity-information`  -  Query entity information
- `autotask-cli departments query-field-definitions`  -  Query field definitions
- `autotask-cli departments query-item`  -  Query item
- `autotask-cli departments query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli departments update-entity`  -  Update entity
- `autotask-cli departments url-parameter-query`  -  Url parameter query
- `autotask-cli departments url-parameter-query-count`  -  Url parameter query count

**holidays**  -  Manage holidays

- `autotask-cli holidays query`  -  Query
- `autotask-cli holidays query-count`  -  Query count
- `autotask-cli holidays query-entity-information`  -  Query entity information
- `autotask-cli holidays query-field-definitions`  -  Query field definitions
- `autotask-cli holidays query-item`  -  Query item
- `autotask-cli holidays query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli holidays url-parameter-query`  -  Url parameter query
- `autotask-cli holidays url-parameter-query-count`  -  Url parameter query count

**invoices**  -  Manage invoices

- `autotask-cli invoices patch-entity`  -  Patch entity
- `autotask-cli invoices query`  -  Query
- `autotask-cli invoices query-count`  -  Query count
- `autotask-cli invoices query-entity-information`  -  Query entity information
- `autotask-cli invoices query-field-definitions`  -  Query field definitions
- `autotask-cli invoices query-item`  -  Query item
- `autotask-cli invoices query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli invoices update-entity`  -  Update entity
- `autotask-cli invoices url-parameter-query`  -  Url parameter query
- `autotask-cli invoices url-parameter-query-count`  -  Url parameter query count

**opportunities**  -  Manage opportunities

- `autotask-cli opportunities create-entity`  -  Create entity
- `autotask-cli opportunities patch-entity`  -  Patch entity
- `autotask-cli opportunities query`  -  Query
- `autotask-cli opportunities query-count`  -  Query count
- `autotask-cli opportunities query-entity-information`  -  Query entity information
- `autotask-cli opportunities query-field-definitions`  -  Query field definitions
- `autotask-cli opportunities query-item`  -  Query item
- `autotask-cli opportunities query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli opportunities update-entity`  -  Update entity
- `autotask-cli opportunities url-parameter-query`  -  Url parameter query
- `autotask-cli opportunities url-parameter-query-count`  -  Url parameter query count

**phases**  -  Manage phases

- `autotask-cli phases query`  -  Query
- `autotask-cli phases query-count`  -  Query count
- `autotask-cli phases query-entity-information`  -  Query entity information
- `autotask-cli phases query-field-definitions`  -  Query field definitions
- `autotask-cli phases query-item`  -  Query item
- `autotask-cli phases query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli phases url-parameter-query`  -  Url parameter query
- `autotask-cli phases url-parameter-query-count`  -  Url parameter query count

**products**  -  Manage products

- `autotask-cli products create-entity`  -  Create entity
- `autotask-cli products patch-entity`  -  Patch entity
- `autotask-cli products query`  -  Query
- `autotask-cli products query-count`  -  Query count
- `autotask-cli products query-entity-information`  -  Query entity information
- `autotask-cli products query-field-definitions`  -  Query field definitions
- `autotask-cli products query-item`  -  Query item
- `autotask-cli products query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli products update-entity`  -  Update entity
- `autotask-cli products url-parameter-query`  -  Url parameter query
- `autotask-cli products url-parameter-query-count`  -  Url parameter query count

**project-charges**  -  Manage project charges

- `autotask-cli project-charges query`  -  Query
- `autotask-cli project-charges query-count`  -  Query count
- `autotask-cli project-charges query-entity-information`  -  Query entity information
- `autotask-cli project-charges query-field-definitions`  -  Query field definitions
- `autotask-cli project-charges query-item`  -  Query item
- `autotask-cli project-charges query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli project-charges url-parameter-query`  -  Url parameter query
- `autotask-cli project-charges url-parameter-query-count`  -  Url parameter query count

**project-notes**  -  Manage project notes

- `autotask-cli project-notes query`  -  Query
- `autotask-cli project-notes query-count`  -  Query count
- `autotask-cli project-notes query-entity-information`  -  Query entity information
- `autotask-cli project-notes query-field-definitions`  -  Query field definitions
- `autotask-cli project-notes query-item`  -  Query item
- `autotask-cli project-notes query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli project-notes url-parameter-query`  -  Url parameter query
- `autotask-cli project-notes url-parameter-query-count`  -  Url parameter query count

**projects**  -  Manage projects

- `autotask-cli projects create-entity`  -  Create entity
- `autotask-cli projects patch-entity`  -  Patch entity
- `autotask-cli projects query`  -  Query
- `autotask-cli projects query-count`  -  Query count
- `autotask-cli projects query-entity-information`  -  Query entity information
- `autotask-cli projects query-field-definitions`  -  Query field definitions
- `autotask-cli projects query-item`  -  Query item
- `autotask-cli projects query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli projects update-entity`  -  Update entity
- `autotask-cli projects url-parameter-query`  -  Url parameter query
- `autotask-cli projects url-parameter-query-count`  -  Url parameter query count

**purchase-orders**  -  Manage purchase orders

- `autotask-cli purchase-orders create-entity`  -  Create entity
- `autotask-cli purchase-orders patch-entity`  -  Patch entity
- `autotask-cli purchase-orders query`  -  Query
- `autotask-cli purchase-orders query-count`  -  Query count
- `autotask-cli purchase-orders query-entity-information`  -  Query entity information
- `autotask-cli purchase-orders query-field-definitions`  -  Query field definitions
- `autotask-cli purchase-orders query-item`  -  Query item
- `autotask-cli purchase-orders query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli purchase-orders update-entity`  -  Update entity
- `autotask-cli purchase-orders url-parameter-query`  -  Url parameter query
- `autotask-cli purchase-orders url-parameter-query-count`  -  Url parameter query count

**quotes**  -  Manage quotes

- `autotask-cli quotes create-entity`  -  Create entity
- `autotask-cli quotes patch-entity`  -  Patch entity
- `autotask-cli quotes query`  -  Query
- `autotask-cli quotes query-count`  -  Query count
- `autotask-cli quotes query-entity-information`  -  Query entity information
- `autotask-cli quotes query-field-definitions`  -  Query field definitions
- `autotask-cli quotes query-item`  -  Query item
- `autotask-cli quotes query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli quotes update-entity`  -  Update entity
- `autotask-cli quotes url-parameter-query`  -  Url parameter query
- `autotask-cli quotes url-parameter-query-count`  -  Url parameter query count

**resource-roles**  -  Manage resource roles

- `autotask-cli resource-roles query`  -  Query
- `autotask-cli resource-roles query-count`  -  Query count
- `autotask-cli resource-roles query-entity-information`  -  Query entity information
- `autotask-cli resource-roles query-field-definitions`  -  Query field definitions
- `autotask-cli resource-roles query-item`  -  Query item
- `autotask-cli resource-roles query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli resource-roles url-parameter-query`  -  Url parameter query
- `autotask-cli resource-roles url-parameter-query-count`  -  Url parameter query count

**resources**  -  Manage resources

- `autotask-cli resources patch-entity`  -  Patch entity
- `autotask-cli resources query`  -  Query
- `autotask-cli resources query-count`  -  Query count
- `autotask-cli resources query-entity-information`  -  Query entity information
- `autotask-cli resources query-field-definitions`  -  Query field definitions
- `autotask-cli resources query-item`  -  Query item
- `autotask-cli resources query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli resources update-entity`  -  Update entity
- `autotask-cli resources url-parameter-query`  -  Url parameter query
- `autotask-cli resources url-parameter-query-count`  -  Url parameter query count

**roles**  -  Manage roles

- `autotask-cli roles create-entity`  -  Create entity
- `autotask-cli roles patch-entity`  -  Patch entity
- `autotask-cli roles query`  -  Query
- `autotask-cli roles query-count`  -  Query count
- `autotask-cli roles query-entity-information`  -  Query entity information
- `autotask-cli roles query-field-definitions`  -  Query field definitions
- `autotask-cli roles query-item`  -  Query item
- `autotask-cli roles query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli roles update-entity`  -  Update entity
- `autotask-cli roles url-parameter-query`  -  Url parameter query
- `autotask-cli roles url-parameter-query-count`  -  Url parameter query count

**service-bundles**  -  Manage service bundles

- `autotask-cli service-bundles create-entity`  -  Create entity
- `autotask-cli service-bundles delete-entity`  -  Delete entity
- `autotask-cli service-bundles patch-entity`  -  Patch entity
- `autotask-cli service-bundles query`  -  Query
- `autotask-cli service-bundles query-count`  -  Query count
- `autotask-cli service-bundles query-entity-information`  -  Query entity information
- `autotask-cli service-bundles query-field-definitions`  -  Query field definitions
- `autotask-cli service-bundles query-item`  -  Query item
- `autotask-cli service-bundles query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli service-bundles update-entity`  -  Update entity
- `autotask-cli service-bundles url-parameter-query`  -  Url parameter query
- `autotask-cli service-bundles url-parameter-query-count`  -  Url parameter query count

**service-calls**  -  Manage service calls

- `autotask-cli service-calls create-entity`  -  Create entity
- `autotask-cli service-calls delete-entity`  -  Delete entity
- `autotask-cli service-calls patch-entity`  -  Patch entity
- `autotask-cli service-calls query`  -  Query
- `autotask-cli service-calls query-count`  -  Query count
- `autotask-cli service-calls query-entity-information`  -  Query entity information
- `autotask-cli service-calls query-field-definitions`  -  Query field definitions
- `autotask-cli service-calls query-item`  -  Query item
- `autotask-cli service-calls query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli service-calls update-entity`  -  Update entity
- `autotask-cli service-calls url-parameter-query`  -  Url parameter query
- `autotask-cli service-calls url-parameter-query-count`  -  Url parameter query count

**services**  -  Manage services

- `autotask-cli services create-entity`  -  Create entity
- `autotask-cli services patch-entity`  -  Patch entity
- `autotask-cli services query`  -  Query
- `autotask-cli services query-count`  -  Query count
- `autotask-cli services query-entity-information`  -  Query entity information
- `autotask-cli services query-field-definitions`  -  Query field definitions
- `autotask-cli services query-item`  -  Query item
- `autotask-cli services query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli services update-entity`  -  Update entity
- `autotask-cli services url-parameter-query`  -  Url parameter query
- `autotask-cli services url-parameter-query-count`  -  Url parameter query count

**skills**  -  Manage skills

- `autotask-cli skills query`  -  Query
- `autotask-cli skills query-count`  -  Query count
- `autotask-cli skills query-entity-information`  -  Query entity information
- `autotask-cli skills query-field-definitions`  -  Query field definitions
- `autotask-cli skills query-item`  -  Query item
- `autotask-cli skills query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli skills url-parameter-query`  -  Url parameter query
- `autotask-cli skills url-parameter-query-count`  -  Url parameter query count

**subscriptions**  -  Manage subscriptions

- `autotask-cli subscriptions create-entity`  -  Create entity
- `autotask-cli subscriptions delete-entity`  -  Delete entity
- `autotask-cli subscriptions patch-entity`  -  Patch entity
- `autotask-cli subscriptions query`  -  Query
- `autotask-cli subscriptions query-count`  -  Query count
- `autotask-cli subscriptions query-entity-information`  -  Query entity information
- `autotask-cli subscriptions query-field-definitions`  -  Query field definitions
- `autotask-cli subscriptions query-item`  -  Query item
- `autotask-cli subscriptions query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli subscriptions update-entity`  -  Update entity
- `autotask-cli subscriptions url-parameter-query`  -  Url parameter query
- `autotask-cli subscriptions url-parameter-query-count`  -  Url parameter query count

**task-notes**  -  Manage task notes

- `autotask-cli task-notes create-entity`  -  Create entity
- `autotask-cli task-notes patch-entity`  -  Patch entity
- `autotask-cli task-notes query`  -  Query
- `autotask-cli task-notes query-count`  -  Query count
- `autotask-cli task-notes query-entity-information`  -  Query entity information
- `autotask-cli task-notes query-field-definitions`  -  Query field definitions
- `autotask-cli task-notes query-item`  -  Query item
- `autotask-cli task-notes query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli task-notes update-entity`  -  Update entity
- `autotask-cli task-notes url-parameter-query`  -  Url parameter query
- `autotask-cli task-notes url-parameter-query-count`  -  Url parameter query count

**tasks**  -  Manage tasks

- `autotask-cli tasks query`  -  Query
- `autotask-cli tasks query-count`  -  Query count
- `autotask-cli tasks query-entity-information`  -  Query entity information
- `autotask-cli tasks query-field-definitions`  -  Query field definitions
- `autotask-cli tasks query-item`  -  Query item
- `autotask-cli tasks query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli tasks url-parameter-query`  -  Url parameter query
- `autotask-cli tasks url-parameter-query-count`  -  Url parameter query count

**ticket-categories**  -  Manage ticket categories

- `autotask-cli ticket-categories patch-entity`  -  Patch entity
- `autotask-cli ticket-categories query`  -  Query
- `autotask-cli ticket-categories query-count`  -  Query count
- `autotask-cli ticket-categories query-entity-information`  -  Query entity information
- `autotask-cli ticket-categories query-field-definitions`  -  Query field definitions
- `autotask-cli ticket-categories query-item`  -  Query item
- `autotask-cli ticket-categories query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli ticket-categories update-entity`  -  Update entity
- `autotask-cli ticket-categories url-parameter-query`  -  Url parameter query
- `autotask-cli ticket-categories url-parameter-query-count`  -  Url parameter query count

**ticket-charges**  -  Manage ticket charges

- `autotask-cli ticket-charges query`  -  Query
- `autotask-cli ticket-charges query-count`  -  Query count
- `autotask-cli ticket-charges query-entity-information`  -  Query entity information
- `autotask-cli ticket-charges query-field-definitions`  -  Query field definitions
- `autotask-cli ticket-charges query-item`  -  Query item
- `autotask-cli ticket-charges query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli ticket-charges url-parameter-query`  -  Url parameter query
- `autotask-cli ticket-charges url-parameter-query-count`  -  Url parameter query count

**ticket-history**  -  Manage ticket history

- `autotask-cli ticket-history query`  -  Query
- `autotask-cli ticket-history query-count`  -  Query count
- `autotask-cli ticket-history query-entity-information`  -  Query entity information
- `autotask-cli ticket-history query-field-definitions`  -  Query field definitions
- `autotask-cli ticket-history query-item`  -  Query item
- `autotask-cli ticket-history query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli ticket-history url-parameter-query`  -  Url parameter query
- `autotask-cli ticket-history url-parameter-query-count`  -  Url parameter query count

**ticket-notes**  -  Manage ticket notes

- `autotask-cli ticket-notes query`  -  Query
- `autotask-cli ticket-notes query-count`  -  Query count
- `autotask-cli ticket-notes query-entity-information`  -  Query entity information
- `autotask-cli ticket-notes query-field-definitions`  -  Query field definitions
- `autotask-cli ticket-notes query-item`  -  Query item
- `autotask-cli ticket-notes query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli ticket-notes url-parameter-query`  -  Url parameter query
- `autotask-cli ticket-notes url-parameter-query-count`  -  Url parameter query count

**ticket-secondary-resources**  -  Manage ticket secondary resources

- `autotask-cli ticket-secondary-resources query`  -  Query
- `autotask-cli ticket-secondary-resources query-count`  -  Query count
- `autotask-cli ticket-secondary-resources query-entity-information`  -  Query entity information
- `autotask-cli ticket-secondary-resources query-field-definitions`  -  Query field definitions
- `autotask-cli ticket-secondary-resources query-item`  -  Query item
- `autotask-cli ticket-secondary-resources query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli ticket-secondary-resources url-parameter-query`  -  Url parameter query
- `autotask-cli ticket-secondary-resources url-parameter-query-count`  -  Url parameter query count

**tickets**  -  Manage tickets

- `autotask-cli tickets create-entity`  -  Create entity
- `autotask-cli tickets patch-entity`  -  Patch entity
- `autotask-cli tickets query`  -  Query
- `autotask-cli tickets query-count`  -  Query count
- `autotask-cli tickets query-entity-information`  -  Query entity information
- `autotask-cli tickets query-field-definitions`  -  Query field definitions
- `autotask-cli tickets query-item`  -  Query item
- `autotask-cli tickets query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli tickets update-entity`  -  Update entity
- `autotask-cli tickets url-parameter-query`  -  Url parameter query
- `autotask-cli tickets url-parameter-query-count`  -  Url parameter query count

**time-entries**  -  Manage time entries

- `autotask-cli time-entries create-entity`  -  Create entity
- `autotask-cli time-entries delete-entity`  -  Delete entity
- `autotask-cli time-entries patch-entity`  -  Patch entity
- `autotask-cli time-entries query`  -  Query
- `autotask-cli time-entries query-count`  -  Query count
- `autotask-cli time-entries query-entity-information`  -  Query entity information
- `autotask-cli time-entries query-field-definitions`  -  Query field definitions
- `autotask-cli time-entries query-item`  -  Query item
- `autotask-cli time-entries query-user-defined-field-definitions`  -  Query user defined field definitions
- `autotask-cli time-entries update-entity`  -  Update entity
- `autotask-cli time-entries url-parameter-query`  -  Url parameter query
- `autotask-cli time-entries url-parameter-query-count`  -  Url parameter query count


**account-brief**


- `autotask-cli account-brief`  -  What changed on one account since a point in time  -  new tickets, contract burn movement, opportunities, stale items.


**company-360**


- `autotask-cli company-360`  -  One view of a company's tickets, contacts, contracts, config items, and opportunities.


**contract-burn**


- `autotask-cli contract-burn`  -  Show how much of each contract's hours/blocks are consumed versus purchased.


**data-gaps**


- `autotask-cli data-gaps`  -  Find tickets with no contract, time entries with no ticket, contacts and config items with no company.


**picklist**


- `autotask-cli picklist`  -  Print the label-to-ID map for any picklist field (status, priority, queue) from cached field metadata.


**project-health**


- `autotask-cli project-health`  -  Flag projects with overdue tasks or past end dates.


**reconcile**


- `autotask-cli reconcile`  -  One month-end table: billable time not yet approved and posted, contract blocks consumed vs purchased, and the money left on the table.


**retainer**


- `autotask-cli retainer`  -  Block-hour contracts ranked by percent consumed, with projected run-out dates from recent burn rate.


**since**


- `autotask-cli since`  -  See what changed across tickets in the last N hours or days.


**sla-breaches**


- `autotask-cli sla-breaches`  -  List open tickets past their due date or overdue for first response.


**stale**


- `autotask-cli stale`  -  Find tickets or projects with no activity in N days.


**ticket-aging**


- `autotask-cli ticket-aging`  -  Bucket open tickets by age so you see how stale the service desk is at a glance.


**triage**


- `autotask-cli triage`  -  Rank open unassigned tickets by priority and age into a workable queue.


**unbilled**


- `autotask-cli unbilled`  -  Surface billable time entries not yet approved and posted for billing  -  money on the table.


**workload**


- `autotask-cli workload`  -  See which technicians are overloaded by open ticket and task hours before you assign more.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
autotask-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Morning dispatch

```bash
autotask-cli triage --agent
```

Open unassigned tickets ranked by priority and age.

### Service-desk health

```bash
autotask-cli ticket-aging --by queue --agent
```

Open tickets bucketed by age, broken down by queue.

### Find money on the table

```bash
autotask-cli unbilled --agent
```

Approved time entries not yet invoiced.

### Account review

```bash
autotask-cli company-360 1234 --agent
```

One company's tickets, contacts, contracts, config items, and opportunities.

### Offline search

```bash
autotask-cli search "vpn outage" --type tickets
```

Full-text search across synced tickets with no API call.

## Auth Setup

Autotask uses three headers on every call: ApiIntegrationCode, UserName, and Secret (set AUTOTASK_API_INTEGRATION_CODE, AUTOTASK_PSA_USER_NAME, AUTOTASK_PSA_SECRET). Your tenant lives in a numbered zone; run `autotask-cli zone --user <api-user-email>` once to discover and cache the zone base URL (or set AUTOTASK_BASE_URL).

Run `autotask-cli doctor` to verify setup.

## Credential safety

- The three Autotask headers (`ApiIntegrationCode`, `UserName`, `Secret`) and every API request go only to the scheme, host and port of the configured base URL (`AUTOTASK_BASE_URL`, default `https://webservices5.autotask.net/atservicesrest/V1.0`). A redirect to any other host is refused, so the credentials never reach it. `zone` asks only `webservices.autotask.net` and saves only an `https://webservicesN.autotask.net/atservicesrest` zone URL.
- Any output line or JSON document, file write, local store row or MCP result that would repeat a credential this program holds (`AUTOTASK_API_INTEGRATION_CODE`, `AUTOTASK_PSA_SECRET`, `AUTOTASK_MCP_HTTP_TOKEN`, the stored integration code, or one in `config.toml`) is withheld whole, in any encoding, and "[credkit: withheld from stdout: it carried a credential this program holds]" appears in its place (naming stderr, mcp, file, sqlite, child process or outbound request where that is the sink). A credential-named field in a response (a webhook `secretKey`, a client portal user's `password`, an integration vendor's `secret`) is replaced by `[withheld]`, so the rest of the record stays readable. Responses are never cached on disk.
- The integration code and the Secret are the credentials this program holds. The API user name (`AUTOTASK_PSA_USER_NAME`, sent as `UserName`) is the account's login name and is not treated as a secret: an answer that repeats it or its email domain (Resources, Contacts) prints and syncs as it comes.
- Only `credentials.toml` (mode 0600) holds a credential; `auth set-token` reads the integration code from stdin, never from an argument.
- `--dry-run` sends nothing. Over MCP, a tool that changes something (in Autotask, or this CLI's saved zone) returns a preview and acts only with `confirm: true`, and a tool argument can never name a file to read or write.
- A `--stdin` body is held as a credential, whole and line by line: a preview of it (`--dry-run`) and an answer that repeats part of it are withheld, while a write still sends it. `feedback` takes its note as an argument, not from stdin.
- `config.toml` never keeps the userinfo or query of `base_url`; they stay in `AUTOTASK_BASE_URL`.
- While any credential is configured, `agent-context` (one large JSON document) prints only the withheld notice; run it with no credential configured (for example a fresh `AUTOTASK_HOME` and the credential variables unset) to read it.

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
  autotask-cli appointments query --agent --select createDateTime,creatorResourceID,description
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

- Use `--home <dir>` for one invocation, or set `AUTOTASK_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `AUTOTASK_CONFIG_DIR`, `AUTOTASK_DATA_DIR`, `AUTOTASK_STATE_DIR`, `AUTOTASK_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `AUTOTASK_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `autotask-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "autotask": {
        "command": "autotask-mcp",
        "env": {
          "AUTOTASK_HOME": "/srv/autotask"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `AUTOTASK_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `AUTOTASK_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are never cached on disk. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `AUTOTASK_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `AUTOTASK_DATA_SOURCE` sets the `--data-source` default.

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
autotask-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "autotask-cli learnings confirm 12"] }
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
       materially more, record the divergence via `autotask-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `autotask-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `autotask-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
autotask-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
autotask-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
autotask-cli teach-playbook \
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
autotask-cli playbook amend \
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

`autotask-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `AUTOTASK_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
autotask-cli feedback "the --since flag is inclusive but docs say exclusive"
autotask-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `AUTOTASK_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `AUTOTASK_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
autotask-cli profile save briefing --json
autotask-cli --profile briefing appointments query
autotask-cli profile list --json
autotask-cli profile show briefing
autotask-cli profile delete briefing --yes
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
| 6 | Partial failure |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `autotask-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add autotask-mcp -- autotask-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `autotask-mcp --transport http` requires `AUTOTASK_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `AUTOTASK_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `AUTOTASK_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which autotask-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   autotask-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `autotask-cli <command> --help`.
