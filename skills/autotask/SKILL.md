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
      - kind: go
        bins: [autotask-cli]
        module: github.com/mvanhorn/printing-press-library/library/project-management/autotask/cmd/autotask-cli
---

# Autotask PSA  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `autotask-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install autotask --cli-only
   ```
2. Verify: `autotask-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/project-management/autotask/cmd/autotask-cli@latest
```

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

These capabilities aren't available in any other tool for this API.

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
- **`unbilled`**  -  Surface approved time entries that haven't been invoiced yet  -  money on the table.

  _Use for billing reconciliation and revenue recovery._

  ```bash
  autotask-cli unbilled --company 1234 --agent
  ```
- **`contract-burn`**  -  Show how much of each contract's hours/blocks are consumed versus purchased.

  _Use to spot contracts about to run dry before they do._

  ```bash
  autotask-cli contract-burn --agent
  ```
- **`reconcile`**  -  One month-end table: approved time vs invoiced, contract blocks consumed vs purchased, and the money left on the table.

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
  autotask-cli picklist Tickets status --agent
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


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
autotask-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

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
autotask-cli search "vpn outage"
```

Full-text search across synced tickets and companies with no API call.

## Auth Setup

Autotask uses three headers on every call: ApiIntegrationCode, UserName, and Secret (set AUTOTASK_PSA_API_INTEGRATION_CODE, AUTOTASK_PSA_USER_NAME, AUTOTASK_PSA_SECRET). Your tenant lives in a numbered zone; run `autotask-cli zone --user <api-user-email>` once to discover and cache the zone base URL (or set AUTOTASK_ZONE_URL).

Run `autotask-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  autotask-cli appointments query --agent --select id,name,status
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
autotask-cli feedback "the --since flag is inclusive but docs say exclusive"
autotask-cli feedback --stdin < notes.txt
autotask-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/autotask-cli/feedback.jsonl`. They are never POSTed unless `AUTOTASK_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `AUTOTASK_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `autotask-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/project-management/autotask/cmd/autotask-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add autotask-mcp -- autotask-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which autotask-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   autotask-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `autotask-cli <command> --help`.
