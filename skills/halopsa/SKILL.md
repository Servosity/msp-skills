---
name: halopsa
description: "Every HaloPSA, HaloITSM and HaloCRM feature, plus a local SQLite store and cross-entity views the API can't return. Trigger phrases: `triage my Halo queue`, `check SLA breaches in HaloPSA`, `who is overloaded in Halo`, `client card for Acme in Halo`, `Halo contract burn-down`, `what changed in Halo since this morning`, `find time gaps in my Halo timesheet`, `use halopsa`, `run halopsa`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "HaloPSA"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - halopsa-cli
    install:
      - kind: go
        bins: [halopsa-cli]
        module: github.com/mvanhorn/printing-press-library/library/project-management/halopsa/cmd/halopsa-cli
---

# HaloPSA  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `halopsa-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install halopsa --cli-only
   ```
2. Verify: `halopsa-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/project-management/halopsa/cmd/halopsa-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Wraps the full Halo REST API (952 endpoints across tickets, clients, assets, contracts, time, KB, and workflows) with offline-first search, agent-native JSON output, and cross-entity commands like `triage`, `client card`, and `contracts burn` that join tables Halo's UI scatters across five tabs.

## When to Use This CLI

Reach for halopsa-cli when an agent needs to triage, dispatch, or report against a HaloPSA / HaloITSM / HaloCRM tenant without clicking through the web UI. It is the right tool for cross-entity questions ("who's overloaded", "which tickets are about to breach", "which client is burning their contract"), bulk operations (stale-ticket close, batch action posts, batch time entries), and ETLs that previously required hand-rolled scripts. It is NOT the right tool for end-user portal browsing or for tenants you don't have API credentials to.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Cross-entity dispatch views
- **`triage`**  -  See per-agent open ticket load, stale tickets, and 24-hour SLA-breach count in one table  -  the dispatcher view Halo's UI scatters across five tabs.

  _Reach for this when an agent asks 'who should I assign this P1 to' or 'where are we bleeding'. One call, one screen._

  ```bash
  halopsa-cli triage --team Support --json
  ```
- **`tickets age-out`**  -  Find tickets stale in a status for N days, preview them, then bulk-close with a templated action via --apply.

  _Reach for this on the Monday queue cleanse to close 30 stale tickets in one command instead of 30 clicks._

  ```bash
  halopsa-cli tickets age-out --status "Awaiting Customer" --stale-days 14 --action-note "Auto-closing per policy" --apply
  ```
- **`sla breaching`**  -  List tickets whose targetdate falls in the next N hours, sorted by time-to-breach, with agent + client + current status.

  _Reach for this on Friday afternoon or anytime before a hand-off to pre-empt SLA breaches._

  ```bash
  halopsa-cli sla breaching --within 24h --team Support --json
  ```
- **`agent workload`**  -  Per-agent: open tickets, tickets touched this week, billable hours logged, oldest open ticket age.

  _Reach for this when rebalancing the queue or asking 'who's overloaded'._

  ```bash
  halopsa-cli agent workload --team Support --json
  ```

### Per-client situational awareness
- **`client card`**  -  One panel: client + sites + active tickets + open contracts + contract hours remaining + recent KB articles linked to their tickets + asset count.

  _Reach for this on every client call. Open it before answering the phone, paste it into ticket notes._

  ```bash
  halopsa-cli client card "Acme Corp" --json
  ```
- **`asset history`**  -  Every ticket that touched this asset, chronological, with agent and time logged.

  _Reach for this when a machine keeps coming back to the queue  -  the pattern lives in the history, not in the latest ticket._

  ```bash
  halopsa-cli asset history LAP-0042 --json
  ```
- **`kbarticle suggest`**  -  FTS5-rank KB articles against a ticket's summary + details + last action text; print top 5 with snippets.

  _Reach for this mid-call when a known fix probably exists but you don't remember the exact KB title._

  ```bash
  halopsa-cli kbarticle suggest --ticket 12345 --limit 5
  ```

### Local-only analytics
- **`time gaps`**  -  List tickets the agent touched this week that have zero time logged on them.

  _Reach for this on Friday before submitting the timesheet. Stops 'I know I worked that ticket but where is it' archaeology._

  ```bash
  halopsa-cli time gaps --agent=me --week current
  ```
- **`contracts burn`**  -  Per contract: hours bank, hours consumed this period (sum of billable time on that client's tickets), days remaining, projected overage.

  _Reach for this mid-month before a client conversation  -  know whether they're tracking over their bank._

  ```bash
  halopsa-cli contracts burn --client "Acme Corp" --month current --json
  ```
- **`rules dump`**  -  Print every ticket rule and workflow as readable flat text  -  conditions → actions, one block per rule.

  _Reach for this during quarterly automation audits or when investigating an unexpected routing._

  ```bash
  halopsa-cli rules dump --workflow "New Ticket" > rules-audit.txt
  ```
- **`tickets changed-since`**  -  Tickets where any action or status change occurred since timestamp, grouped by ticket.

  _Reach for this after a meeting or when coming back from lunch. 'What did I miss' in one call._

  ```bash
  halopsa-cli tickets changed-since 09:00 --mine --json
  ```
- **`standup`**  -  Per-agent for the window: tickets closed, tickets reopened, time logged, top client.

  _Reach for this the moment before standup. One paste, everyone sees yesterday's progress._

  ```bash
  halopsa-cli standup --team Support --since yesterday
  ```
- **`client overlay`**  -  Rank all clients by a chosen metric (open tickets, stale, SLA at-risk, hours over bank). Top N out.

  _Reach for this when looking for the next escalation to invest time in. Whichever client is on top is the one to call._

  ```bash
  halopsa-cli client overlay --metric open_tickets --top 10 --json
  ```

### Local state that compounds
- **`time leaks`**  -  Billable time entries not yet attached to any invoice, summed by client and agent  -  the revenue sitting un-invoiced.

  _Reach for this in Monday billing prep to catch revenue leaks before invoices go out._

  ```bash
  halopsa-cli time leaks --month current --json
  ```

### Reporting that writes itself
- **`sla scorecard`**  -  Historical SLA pass-rate for closed tickets  -  % met resolution targets, by team or agent.

  _Reach for this for the weekly leadership report instead of a brittle export pipeline._

  ```bash
  halopsa-cli sla scorecard --since 30d --by team
  ```

### Cross-entity intelligence
- **`assets expiring`**  -  Assets whose linked contract ends in the next N days, joined to the owning client and sorted by days-to-expiry.

  _Reach for this in renewal prep and proactive replacement planning._

  ```bash
  halopsa-cli assets expiring --within 60
  ```
- **`tickets reopens`**  -  Tickets that bounced from closed back to open in the window, grouped by agent and client with reopen counts.

  _Reach for this in quality audits to find boomerang patterns by agent or client._

  ```bash
  halopsa-cli tickets reopens --since 30d
  ```

## Command Reference

**actions**  -  Manage actions

- `halopsa-cli actions create`  -  Create
- `halopsa-cli actions create-reaction`  -  Create reaction
- `halopsa-cli actions create-review`  -  Create review
- `halopsa-cli actions delete`  -  Delete
- `halopsa-cli actions get`  -  Use this to return a single instance of Actions. 				Requires authentication.
- `halopsa-cli actions list`  -  Use this to return multiple Actions. 				Requires authentication.

**addigy**  -  Manage addigy

- `halopsa-cli addigy create`  -  Create
- `halopsa-cli addigy list`  -  List

**addigy-details**  -  Manage addigy details

- `halopsa-cli addigy-details create`  -  Create
- `halopsa-cli addigy-details delete`  -  Delete
- `halopsa-cli addigy-details get`  -  Get
- `halopsa-cli addigy-details list`  -  List

**address**  -  Manage address

- `halopsa-cli address create`  -  Create
- `halopsa-cli address delete`  -  Delete
- `halopsa-cli address get`  -  Use this to return a single instance of AddressStore. 				Requires authentication.
- `halopsa-cli address list`  -  Use this to return multiple AddressStore. 				Requires authentication.

**addressbook**  -  Manage addressbook

- `halopsa-cli addressbook create`  -  Create
- `halopsa-cli addressbook delete`  -  Delete
- `halopsa-cli addressbook get`  -  Get
- `halopsa-cli addressbook list`  -  List

**adobe-acrobat-details**  -  Manage adobe acrobat details

- `halopsa-cli adobe-acrobat-details create`  -  Create
- `halopsa-cli adobe-acrobat-details delete`  -  Delete
- `halopsa-cli adobe-acrobat-details get`  -  Get
- `halopsa-cli adobe-acrobat-details list`  -  List

**adobe-commerce-details**  -  Manage adobe commerce details

- `halopsa-cli adobe-commerce-details create`  -  Create
- `halopsa-cli adobe-commerce-details delete`  -  Delete
- `halopsa-cli adobe-commerce-details get`  -  Get
- `halopsa-cli adobe-commerce-details list`  -  List

**adobe-commerce-integration**  -  Manage adobe commerce integration

- `halopsa-cli adobe-commerce-integration create`  -  Create
- `halopsa-cli adobe-commerce-integration list`  -  List

**agent**  -  Manage agent

- `halopsa-cli agent create`  -  Create
- `halopsa-cli agent create-clearcache`  -  Create clearcache
- `halopsa-cli agent delete`  -  Delete
- `halopsa-cli agent get`  -  Use this to return a single instance of Uname. 				Requires authentication.
- `halopsa-cli agent list`  -  Use this to return multiple Uname. 				Requires authentication.
- `halopsa-cli agent list-me`  -  List me

**agent-check-in**  -  Manage agent check in

- `halopsa-cli agent-check-in create`  -  Create
- `halopsa-cli agent-check-in get`  -  Use this to return a single instance of AgentCheckIn. 				Requires authentication.
- `halopsa-cli agent-check-in list`  -  Use this to return multiple AgentCheckIn. 				Requires authentication.

**agent-event-subscription**  -  Manage agent event subscription

- `halopsa-cli agent-event-subscription create`  -  Create
- `halopsa-cli agent-event-subscription delete`  -  Delete
- `halopsa-cli agent-event-subscription get`  -  Get
- `halopsa-cli agent-event-subscription list`  -  List

**agent-image**  -  Manage agent image

- `halopsa-cli agent-image <id>`  -  Use this to return a single instance of Uname. 				Requires authentication.

**agent-presence-rule**  -  Manage agent presence rule

- `halopsa-cli agent-presence-rule`  -  List

**agent-presence-subscription**  -  Manage agent presence subscription

- `halopsa-cli agent-presence-subscription create`  -  Create
- `halopsa-cli agent-presence-subscription delete`  -  Delete
- `halopsa-cli agent-presence-subscription get-uname-presence-subscription`  -  Get uname presence subscription
- `halopsa-cli agent-presence-subscription list`  -  List

**aisuggestion**  -  Manage aisuggestion

- `halopsa-cli aisuggestion create`  -  Create
- `halopsa-cli aisuggestion delete`  -  Delete
- `halopsa-cli aisuggestion get`  -  Get
- `halopsa-cli aisuggestion list`  -  List

**alemba**  -  Manage alemba

- `halopsa-cli alemba`  -  List

**amazon-seller-details**  -  Manage amazon seller details

- `halopsa-cli amazon-seller-details create`  -  Create
- `halopsa-cli amazon-seller-details delete`  -  Delete
- `halopsa-cli amazon-seller-details get`  -  Get
- `halopsa-cli amazon-seller-details list`  -  List

**application**  -  Manage application

- `halopsa-cli application create`  -  Create
- `halopsa-cli application create-federatedcredentials`  -  Create federatedcredentials
- `halopsa-cli application delete`  -  Delete
- `halopsa-cli application get`  -  Use this to return a single instance of NHD_Identity_Application. 				Requires authentication.
- `halopsa-cli application list`  -  List

**appointment**  -  Manage appointment

- `halopsa-cli appointment create`  -  Create
- `halopsa-cli appointment create-booking`  -  Create booking
- `halopsa-cli appointment create-generate`  -  Create generate
- `halopsa-cli appointment delete`  -  Delete specific Appointment. 				Requires authentication.
- `halopsa-cli appointment get`  -  Use this to return a single instance of Appointment. 				Requires authentication.
- `halopsa-cli appointment list`  -  Use this to return multiple Appointment. 				Requires authentication.
- `halopsa-cli appointment list-booking`  -  List booking

**approval-process**  -  Manage approval process

- `halopsa-cli approval-process create`  -  Create
- `halopsa-cli approval-process delete`  -  Delete
- `halopsa-cli approval-process get`  -  Use this to return a single instance of ApprovalProcess. 				Requires authentication.
- `halopsa-cli approval-process list`  -  Use this to return multiple ApprovalProcess. 				Requires authentication.

**approval-process-rule**  -  Manage approval process rule

- `halopsa-cli approval-process-rule create`  -  Create
- `halopsa-cli approval-process-rule delete`  -  Delete
- `halopsa-cli approval-process-rule get`  -  Use this to return a single instance of ApprovalProcessRule. 				Requires authentication.
- `halopsa-cli approval-process-rule list`  -  Use this to return multiple ApprovalProcessRule. 				Requires authentication.

**area-azure-tenant**  -  Manage area azure tenant

- `halopsa-cli area-azure-tenant`  -  Use this to return multiple AreaAzureTenant. 				Requires authentication.

**area-request-type**  -  Manage area request type

- `halopsa-cli area-request-type get`  -  Use this to return a single instance of AreaRequestType. 				Requires authentication.
- `halopsa-cli area-request-type list`  -  List

**armis**  -  Manage armis

- `halopsa-cli armis`  -  List

**armis-details**  -  Manage armis details

- `halopsa-cli armis-details create`  -  Create
- `halopsa-cli armis-details delete`  -  Delete
- `halopsa-cli armis-details get`  -  Get
- `halopsa-cli armis-details list`  -  List

**arrow-sphere-details**  -  Manage arrow sphere details

- `halopsa-cli arrow-sphere-details create`  -  Create
- `halopsa-cli arrow-sphere-details delete`  -  Delete
- `halopsa-cli arrow-sphere-details get`  -  Get
- `halopsa-cli arrow-sphere-details list`  -  List

**asset**  -  Manage asset

- `halopsa-cli asset create`  -  Create
- `halopsa-cli asset delete`  -  Delete
- `halopsa-cli asset get`  -  Use this to return a single instance of Device. 				Requires authentication.
- `halopsa-cli asset list`  -  Use this to return multiple Device. 				Requires authentication.
- `halopsa-cli asset list-getallsoftwareversions`  -  List getallsoftwareversions
- `halopsa-cli asset list-nexttag`  -  List nexttag

**asset-change**  -  Manage asset change

- `halopsa-cli asset-change create`  -  Create
- `halopsa-cli asset-change list`  -  Use this to return multiple DeviceChange. 				Requires authentication.

**asset-group**  -  Manage asset group

- `halopsa-cli asset-group create`  -  Create
- `halopsa-cli asset-group delete`  -  Delete
- `halopsa-cli asset-group get`  -  Use this to return a single instance of Generic. 				Requires authentication.
- `halopsa-cli asset-group list`  -  Use this to return multiple Generic. 				Requires authentication.

**asset-software**  -  Manage asset software

- `halopsa-cli asset-software`  -  Use this to return multiple DeviceApplications. 				Requires authentication.

**asset-type**  -  Manage asset type

- `halopsa-cli asset-type create`  -  Create
- `halopsa-cli asset-type delete`  -  Delete
- `halopsa-cli asset-type get`  -  Use this to return a single instance of Xtype. 				Requires authentication.
- `halopsa-cli asset-type list`  -  Use this to return multiple Xtype. 				Requires authentication.

**asset-type-info**  -  Manage asset type info

- `halopsa-cli asset-type-info`  -  Use this to return multiple Xtype. 				Requires authentication.

**asset-type-mappings**  -  Manage asset type mappings

- `halopsa-cli asset-type-mappings get`  -  Use this to return a single instance of XTypeMapping. 				Requires authentication.
- `halopsa-cli asset-type-mappings list`  -  List

**att**  -  Manage att

- `halopsa-cli att`  -  List

**attachment**  -  Manage attachment

- `halopsa-cli attachment create`  -  Create
- `halopsa-cli attachment create-document`  -  Create document
- `halopsa-cli attachment create-gets3presignedurl`  -  Create gets3presignedurl
- `halopsa-cli attachment create-image`  -  Create image
- `halopsa-cli attachment create-presignedurluploadcomplete`  -  Create presignedurluploadcomplete
- `halopsa-cli attachment delete`  -  Delete
- `halopsa-cli attachment delete-document`  -  Delete document
- `halopsa-cli attachment delete-image`  -  Delete image
- `halopsa-cli attachment get`  -  Use this to return a single instance of Attachment. 				Requires authentication.
- `halopsa-cli attachment get-document`  -  Get document
- `halopsa-cli attachment get-image`  -  Get image
- `halopsa-cli attachment get-nhserver`  -  Get nhserver
- `halopsa-cli attachment list`  -  Use this to return multiple Attachment. 				Requires authentication.
- `halopsa-cli attachment list-image`  -  List image

**audit**  -  Manage audit

- `halopsa-cli audit create`  -  Create
- `halopsa-cli audit delete`  -  Delete
- `halopsa-cli audit get`  -  Use this to return a single instance of Audit. 				Requires authentication.
- `halopsa-cli audit list`  -  List

**auth-info**  -  Manage auth info

- `halopsa-cli auth-info`  -  List

**automation**  -  Manage automation

- `halopsa-cli automation create`  -  Create
- `halopsa-cli automation create-runbookid`  -  Create runbookid
- `halopsa-cli automation delete`  -  Delete
- `halopsa-cli automation get`  -  Get
- `halopsa-cli automation list`  -  List

**avalara-details**  -  Manage avalara details

- `halopsa-cli avalara-details create`  -  Create
- `halopsa-cli avalara-details delete`  -  Delete
- `halopsa-cli avalara-details get`  -  Get
- `halopsa-cli avalara-details list`  -  List

**aws**  -  Manage aws

- `halopsa-cli aws`  -  List

**awsdetails**  -  Manage awsdetails

- `halopsa-cli awsdetails create`  -  Create
- `halopsa-cli awsdetails delete`  -  Delete
- `halopsa-cli awsdetails get`  -  Get
- `halopsa-cli awsdetails list`  -  List

**azure-delta**  -  Manage azure delta

- `halopsa-cli azure-delta create`  -  Create
- `halopsa-cli azure-delta delete`  -  Delete
- `halopsa-cli azure-delta get`  -  Get
- `halopsa-cli azure-delta list`  -  List

**azure-dev-ops-details**  -  Manage azure dev ops details

- `halopsa-cli azure-dev-ops-details create`  -  Create
- `halopsa-cli azure-dev-ops-details delete`  -  Delete
- `halopsa-cli azure-dev-ops-details get`  -  Use this to return a single instance of AzureDevOpsDetails. 				Requires authentication.
- `halopsa-cli azure-dev-ops-details list`  -  List

**azure-translate**  -  Manage azure translate

- `halopsa-cli azure-translate create`  -  Create
- `halopsa-cli azure-translate list`  -  List

**azureadconnection**  -  Manage azureadconnection

- `halopsa-cli azureadconnection create`  -  Create
- `halopsa-cli azureadconnection delete`  -  Delete
- `halopsa-cli azureadconnection get`  -  Use this to return a single instance of AzureADConnection. 				Requires authentication.
- `halopsa-cli azureadconnection list`  -  Use this to return multiple AzureADConnection. 				Requires authentication.

**azureadmapping**  -  Manage azureadmapping

- `halopsa-cli azureadmapping`  -  Use this to return multiple AzureADMapping. 				Requires authentication.

**background-task**  -  Manage background task

- `halopsa-cli background-task <id>`  -  Get

**billing-template**  -  Manage billing template

- `halopsa-cli billing-template create`  -  Create
- `halopsa-cli billing-template delete`  -  Delete
- `halopsa-cli billing-template get`  -  Use this to return a single instance of ContractTemplateHeader. 				Requires authentication.
- `halopsa-cli billing-template list`  -  List

**booking-type**  -  Manage booking type

- `halopsa-cli booking-type`  -  Use this to return multiple BookingType. 				Requires authentication.

**bookmark**  -  Manage bookmark

- `halopsa-cli bookmark create`  -  Create
- `halopsa-cli bookmark get`  -  Get

**budget-type**  -  Manage budget type

- `halopsa-cli budget-type create`  -  Create
- `halopsa-cli budget-type delete`  -  Delete
- `halopsa-cli budget-type get`  -  Use this to return a single instance of BudgetType. 				Requires authentication.
- `halopsa-cli budget-type list`  -  Use this to return multiple BudgetType. 				Requires authentication.

**bulk-email**  -  Manage bulk email

- `halopsa-cli bulk-email get`  -  Use this to return a single instance of BulkEmail. 				Requires authentication.
- `halopsa-cli bulk-email list`  -  List

**business-central-details**  -  Manage business central details

- `halopsa-cli business-central-details create`  -  Create
- `halopsa-cli business-central-details delete`  -  Delete
- `halopsa-cli business-central-details get`  -  Use this to return a single instance of BusinessCentralDetails. 				Requires authentication.
- `halopsa-cli business-central-details list`  -  Use this to return multiple BusinessCentralDetails. 				Requires authentication.

**cab**  -  Manage cab

- `halopsa-cli cab create`  -  Create
- `halopsa-cli cab delete`  -  Delete
- `halopsa-cli cab get`  -  Use this to return a single instance of CabHeader. 				Requires authentication.
- `halopsa-cli cab list`  -  Use this to return multiple CabHeader. 				Requires authentication.

**cabmember**  -  Manage cabmember

- `halopsa-cli cabmember`  -  List

**cabrole**  -  Manage cabrole

- `halopsa-cli cabrole`  -  List

**call-log**  -  Manage call log

- `halopsa-cli call-log create`  -  Create
- `halopsa-cli call-log get`  -  Use this to return a single instance of CallLog. 				Requires authentication.
- `halopsa-cli call-log list`  -  Use this to return multiple CallLog. 				Requires authentication.

**call-script**  -  Manage call script

- `halopsa-cli call-script create`  -  Create
- `halopsa-cli call-script delete`  -  Delete
- `halopsa-cli call-script get`  -  Use this to return a single instance of ScriptHeader. 				Requires authentication.
- `halopsa-cli call-script list`  -  List

**canned-text**  -  Manage canned text

- `halopsa-cli canned-text create`  -  Create
- `halopsa-cli canned-text create-cannedtext`  -  Create cannedtext
- `halopsa-cli canned-text delete`  -  Delete
- `halopsa-cli canned-text get`  -  Use this to return a single instance of CannedText. 				Requires authentication.
- `halopsa-cli canned-text list`  -  Use this to return multiple CannedText. 				Requires authentication.

**category**  -  Manage category

- `halopsa-cli category create`  -  Create
- `halopsa-cli category delete`  -  Delete
- `halopsa-cli category get`  -  Use this to return a single instance of CategoryDetail. 				Requires authentication.
- `halopsa-cli category list`  -  Use this to return multiple CategoryDetail. 				Requires authentication.

**certificate**  -  Manage certificate

- `halopsa-cli certificate create`  -  Create
- `halopsa-cli certificate delete`  -  Delete
- `halopsa-cli certificate get`  -  Use this to return a single instance of Certificate. 				Requires authentication.
- `halopsa-cli certificate list`  -  List

**change-calendar**  -  Manage change calendar

- `halopsa-cli change-calendar`  -  List

**charge-rate**  -  Manage charge rate

- `halopsa-cli charge-rate get`  -  Use this to return a single instance of ChargeRate. 				Requires authentication.
- `halopsa-cli charge-rate list`  -  Use this to return multiple ChargeRate. 				Requires authentication.

**chat**  -  Manage chat

- `halopsa-cli chat create`  -  Create
- `halopsa-cli chat get`  -  Get
- `halopsa-cli chat list`  -  Use this to return multiple LiveChatHeader. 				Requires authentication.

**chat-flow**  -  Manage chat flow

- `halopsa-cli chat-flow`  -  Create

**chat-matching-data**  -  Manage chat matching data

- `halopsa-cli chat-matching-data`  -  Create

**chat-message**  -  Manage chat message

- `halopsa-cli chat-message create`  -  Create
- `halopsa-cli chat-message create-chatmessage`  -  Create chatmessage
- `halopsa-cli chat-message list`  -  Use this to return multiple LiveChatMsg. 				Requires authentication.

**chat-profile**  -  Manage chat profile

- `halopsa-cli chat-profile create`  -  Create
- `halopsa-cli chat-profile delete`  -  Delete
- `halopsa-cli chat-profile get`  -  Use this to return a single instance of ChatProfile. 				Requires authentication.
- `halopsa-cli chat-profile list`  -  Use this to return multiple ChatProfile. 				Requires authentication.

**client-cache**  -  Manage client cache

- `halopsa-cli client-cache`  -  List

**client-contract**  -  Manage client contract

- `halopsa-cli client-contract create`  -  Create
- `halopsa-cli client-contract create-clientcontract`  -  Create clientcontract
- `halopsa-cli client-contract create-clientcontract-2`  -  Create clientcontract 2
- `halopsa-cli client-contract delete`  -  Delete
- `halopsa-cli client-contract get`  -  Use this to return a single instance of ContractHeader. 				Requires authentication.
- `halopsa-cli client-contract list`  -  Use this to return multiple ContractHeader. 				Requires authentication.

**client-prepay**  -  Manage client prepay

- `halopsa-cli client-prepay create`  -  Create
- `halopsa-cli client-prepay delete`  -  Delete
- `halopsa-cli client-prepay get`  -  Use this to return a single instance of PrepayHistory. 				Requires authentication.
- `halopsa-cli client-prepay list`  -  Use this to return multiple PrepayHistory. 				Requires authentication.

**clients**  -  Manage clients

- `halopsa-cli clients create`  -  Create
- `halopsa-cli clients create-client`  -  Create client
- `halopsa-cli clients create-client-2`  -  Create client 2
- `halopsa-cli clients delete`  -  Delete
- `halopsa-cli clients get`  -  Use this to return a single instance of Area. 				Requires authentication.
- `halopsa-cli clients list`  -  Use this to return multiple Area. 				Requires authentication.
- `halopsa-cli clients list-client`  -  List client

**config-commit**  -  Manage config commit

- `halopsa-cli config-commit create`  -  Create
- `halopsa-cli config-commit delete`  -  Delete
- `halopsa-cli config-commit get`  -  Use this to return a single instance of ConfigCommit. 				Requires authentication.
- `halopsa-cli config-commit list`  -  Use this to return multiple ConfigCommit. 				Requires authentication.

**confirm-closure**  -  Manage confirm closure

- `halopsa-cli confirm-closure create`  -  Create
- `halopsa-cli confirm-closure delete`  -  Delete
- `halopsa-cli confirm-closure get`  -  Use this to return a single instance of ConfirmClosure. 				Requires authentication.
- `halopsa-cli confirm-closure list`  -  List

**confluence-details**  -  Manage confluence details

- `halopsa-cli confluence-details create`  -  Create
- `halopsa-cli confluence-details delete`  -  Delete
- `halopsa-cli confluence-details get`  -  Get
- `halopsa-cli confluence-details list`  -  List

**connected-instance**  -  Manage connected instance

- `halopsa-cli connected-instance create`  -  Create
- `halopsa-cli connected-instance delete`  -  Delete
- `halopsa-cli connected-instance get`  -  Use this to return a single instance of ConnectedInstance. 				Requires authentication.
- `halopsa-cli connected-instance list`  -  List

**consignment**  -  Manage consignment

- `halopsa-cli consignment create`  -  Create
- `halopsa-cli consignment delete`  -  Delete
- `halopsa-cli consignment get`  -  Use this to return a single instance of ConsignmentHeader. 				Requires authentication.
- `halopsa-cli consignment list`  -  Use this to return multiple ConsignmentHeader. 				Requires authentication.

**contactgroup**  -  Manage contactgroup

- `halopsa-cli contactgroup create`  -  Create
- `halopsa-cli contactgroup delete`  -  Delete
- `halopsa-cli contactgroup get`  -  Get
- `halopsa-cli contactgroup list`  -  List

**contactgroupcontact**  -  Manage contactgroupcontact

- `halopsa-cli contactgroupcontact create`  -  Create
- `halopsa-cli contactgroupcontact delete`  -  Delete
- `halopsa-cli contactgroupcontact get`  -  Get
- `halopsa-cli contactgroupcontact list`  -  List

**contract-rule**  -  Manage contract rule

- `halopsa-cli contract-rule create`  -  Create
- `halopsa-cli contract-rule delete`  -  Delete
- `halopsa-cli contract-rule get`  -  Get
- `halopsa-cli contract-rule list`  -  List

**contract-schedule**  -  Manage contract schedule

- `halopsa-cli contract-schedule create`  -  Create
- `halopsa-cli contract-schedule delete`  -  Delete
- `halopsa-cli contract-schedule get`  -  Use this to return a single instance of ContractSchedule. 				Requires authentication.
- `halopsa-cli contract-schedule list`  -  List

**contract-schedule-plan**  -  Manage contract schedule plan

- `halopsa-cli contract-schedule-plan create`  -  Create
- `halopsa-cli contract-schedule-plan delete`  -  Delete
- `halopsa-cli contract-schedule-plan get`  -  Use this to return a single instance of ContractSchedulePlan. 				Requires authentication.
- `halopsa-cli contract-schedule-plan list`  -  List

**cost-centres**  -  Manage cost centres

- `halopsa-cli cost-centres create`  -  Create
- `halopsa-cli cost-centres delete`  -  Delete
- `halopsa-cli cost-centres get`  -  Use this to return a single instance of Costcentres. 				Requires authentication.
- `halopsa-cli cost-centres list`  -  List

**criteria-group**  -  Manage criteria group

- `halopsa-cli criteria-group`  -  List

**crmnote**  -  Manage crmnote

- `halopsa-cli crmnote create`  -  Create
- `halopsa-cli crmnote delete`  -  Delete
- `halopsa-cli crmnote get`  -  Use this to return a single instance of AreaNote. 				Requires authentication.
- `halopsa-cli crmnote list`  -  Use this to return multiple AreaNote. 				Requires authentication.

**cspconsumption-data**  -  Manage cspconsumption data

- `halopsa-cli cspconsumption-data create`  -  Create
- `halopsa-cli cspconsumption-data create-cspconsumptiondata`  -  Create cspconsumptiondata
- `halopsa-cli cspconsumption-data delete`  -  Delete
- `halopsa-cli cspconsumption-data delete-cspconsumptiondata`  -  Delete cspconsumptiondata
- `halopsa-cli cspconsumption-data get`  -  Get
- `halopsa-cli cspconsumption-data list`  -  List

**cspinvoice**  -  Manage cspinvoice

- `halopsa-cli cspinvoice create`  -  Create
- `halopsa-cli cspinvoice delete`  -  Delete
- `halopsa-cli cspinvoice get`  -  Get
- `halopsa-cli cspinvoice list`  -  List

**cspsubscription-pricing**  -  Manage cspsubscription pricing

- `halopsa-cli cspsubscription-pricing`  -  Create

**csvtemplate**  -  Manage csvtemplate

- `halopsa-cli csvtemplate create`  -  Create
- `halopsa-cli csvtemplate delete`  -  Delete
- `halopsa-cli csvtemplate get`  -  Use this to return a single instance of CSVTemplate. 				Requires authentication.
- `halopsa-cli csvtemplate list`  -  List

**currency**  -  Manage currency

- `halopsa-cli currency create`  -  Create
- `halopsa-cli currency delete`  -  Delete
- `halopsa-cli currency get`  -  Use this to return a single instance of Currency. 				Requires authentication.
- `halopsa-cli currency list`  -  List

**custom-button**  -  Manage custom button

- `halopsa-cli custom-button create`  -  Create
- `halopsa-cli custom-button delete`  -  Delete
- `halopsa-cli custom-button get`  -  Use this to return a single instance of CustomButton. 				Requires authentication.
- `halopsa-cli custom-button list`  -  Use this to return multiple CustomButton. 				Requires authentication.

**custom-button-audit**  -  Manage custom button audit

- `halopsa-cli custom-button-audit`  -  Create

**custom-integration**  -  Manage custom integration

- `halopsa-cli custom-integration create`  -  Create
- `halopsa-cli custom-integration delete`  -  Delete
- `halopsa-cli custom-integration get`  -  Use this to return a single instance of OutboundIntegration. 				Requires authentication.
- `halopsa-cli custom-integration list`  -  List

**custom-integration-method**  -  Manage custom integration method

- `halopsa-cli custom-integration-method create`  -  Create
- `halopsa-cli custom-integration-method delete`  -  Delete
- `halopsa-cli custom-integration-method get`  -  Use this to return a single instance of OutboundIntegrationMethod. 				Requires authentication.
- `halopsa-cli custom-integration-method list`  -  Use this to return multiple OutboundIntegrationMethod. 				Requires authentication.

**custom-integration-method-value**  -  Manage custom integration method value

- `halopsa-cli custom-integration-method-value`  -  List

**custom-integration-repository**  -  Manage custom integration repository

- `halopsa-cli custom-integration-repository get`  -  Use this to return a single instance of OutboundIntegration. 				Requires authentication.
- `halopsa-cli custom-integration-repository list`  -  List

**custom-query**  -  Manage custom query

- `halopsa-cli custom-query create`  -  Create
- `halopsa-cli custom-query delete`  -  Delete
- `halopsa-cli custom-query get`  -  Get
- `halopsa-cli custom-query list`  -  List

**custom-table**  -  Manage custom table

- `halopsa-cli custom-table create`  -  Create
- `halopsa-cli custom-table delete`  -  Delete
- `halopsa-cli custom-table get`  -  Use this to return a single instance of CustomTable. 				Requires authentication.
- `halopsa-cli custom-table list`  -  Use this to return multiple CustomTable. 				Requires authentication.

**dashboard-links**  -  Manage dashboard links

- `halopsa-cli dashboard-links create`  -  Create
- `halopsa-cli dashboard-links delete`  -  Delete
- `halopsa-cli dashboard-links get`  -  Use this to return a single instance of DashboardLinks. 				Requires authentication.
- `halopsa-cli dashboard-links list`  -  Use this to return multiple DashboardLinks. 				Requires authentication.
- `halopsa-cli dashboard-links list-dashboardlinks`  -  List dashboardlinks

**dashboard-links-repository**  -  Manage dashboard links repository

- `halopsa-cli dashboard-links-repository get`  -  Use this to return a single instance of DashboardLinks. 				Requires authentication.
- `halopsa-cli dashboard-links-repository list`  -  Use this to return multiple DashboardLinks. 				Requires authentication.

**database-lookup**  -  Manage database lookup

- `halopsa-cli database-lookup create`  -  Create
- `halopsa-cli database-lookup create-databaselookup`  -  Create databaselookup
- `halopsa-cli database-lookup delete`  -  Delete
- `halopsa-cli database-lookup get`  -  Use this to return a single instance of PartsLookup. 				Requires authentication.
- `halopsa-cli database-lookup list`  -  Use this to return multiple PartsLookup. 				Requires authentication.

**database-lookup-confirmation**  -  Manage database lookup confirmation

- `halopsa-cli database-lookup-confirmation create`  -  Create
- `halopsa-cli database-lookup-confirmation get`  -  Get

**datto-commerce-details**  -  Manage datto commerce details

- `halopsa-cli datto-commerce-details create`  -  Create
- `halopsa-cli datto-commerce-details delete`  -  Delete
- `halopsa-cli datto-commerce-details get`  -  Use this to return a single instance of DattoCommerceDetails. 				Requires authentication.
- `halopsa-cli datto-commerce-details list`  -  Use this to return multiple DattoCommerceDetails. 				Requires authentication.

**datto-rmm-details**  -  Manage datto rmm details

- `halopsa-cli datto-rmm-details create`  -  Create
- `halopsa-cli datto-rmm-details delete`  -  Delete
- `halopsa-cli datto-rmm-details get`  -  Get
- `halopsa-cli datto-rmm-details list`  -  List

**device-licence**  -  Manage device licence

- `halopsa-cli device-licence`  -  List

**distribution-lists**  -  Manage distribution lists

- `halopsa-cli distribution-lists create`  -  Create
- `halopsa-cli distribution-lists delete`  -  Delete
- `halopsa-cli distribution-lists get`  -  Get
- `halopsa-cli distribution-lists list`  -  List

**distribution-lists-log**  -  Manage distribution lists log

- `halopsa-cli distribution-lists-log create`  -  Create
- `halopsa-cli distribution-lists-log delete`  -  Delete
- `halopsa-cli distribution-lists-log get`  -  Get
- `halopsa-cli distribution-lists-log list`  -  List

**document-creation**  -  Manage document creation

- `halopsa-cli document-creation`  -  Create

**downtime**  -  Manage downtime

- `halopsa-cli downtime create`  -  Create
- `halopsa-cli downtime delete`  -  Delete
- `halopsa-cli downtime get`  -  Get
- `halopsa-cli downtime list`  -  List
- `halopsa-cli downtime list-downtimecalendar`  -  List downtimecalendar

**draft**  -  Manage draft

- `halopsa-cli draft`  -  Create

**dynamics365-crmdetails**  -  Manage dynamics365 crmdetails

- `halopsa-cli dynamics365-crmdetails create`  -  Create
- `halopsa-cli dynamics365-crmdetails delete`  -  Delete
- `halopsa-cli dynamics365-crmdetails get`  -  Get
- `halopsa-cli dynamics365-crmdetails list`  -  List

**dynatrace-details**  -  Manage dynatrace details

- `halopsa-cli dynatrace-details create`  -  Create
- `halopsa-cli dynatrace-details delete`  -  Delete
- `halopsa-cli dynatrace-details get`  -  Get
- `halopsa-cli dynatrace-details list`  -  List

**ecommerce-order**  -  Manage ecommerce order

- `halopsa-cli ecommerce-order create`  -  Create
- `halopsa-cli ecommerce-order delete`  -  Delete
- `halopsa-cli ecommerce-order get`  -  Get
- `halopsa-cli ecommerce-order list`  -  List

**email-address-book**  -  Manage email address book

- `halopsa-cli email-address-book`  -  Use this to return multiple Users. 				Requires authentication.

**email-rule**  -  Manage email rule

- `halopsa-cli email-rule create`  -  Create
- `halopsa-cli email-rule delete`  -  Delete
- `halopsa-cli email-rule get`  -  Use this to return a single instance of EmailRule. 				Requires authentication.
- `halopsa-cli email-rule list`  -  Use this to return multiple EmailRule. 				Requires authentication.

**email-store**  -  Manage email store

- `halopsa-cli email-store create`  -  Create
- `halopsa-cli email-store delete`  -  Delete
- `halopsa-cli email-store get`  -  Use this to return a single instance of EmailStore. 				Requires authentication.
- `halopsa-cli email-store list`  -  List

**email-template**  -  Manage email template

- `halopsa-cli email-template create`  -  Create
- `halopsa-cli email-template create-emailtemplate`  -  Create emailtemplate
- `halopsa-cli email-template delete`  -  Delete
- `halopsa-cli email-template get`  -  Use this to return a single instance of MessageContent. 				Requires authentication.
- `halopsa-cli email-template list`  -  Use this to return multiple MessageContent. 				Requires authentication.

**email-template-variable**  -  Manage email template variable

- `halopsa-cli email-template-variable create`  -  Create
- `halopsa-cli email-template-variable delete`  -  Delete
- `halopsa-cli email-template-variable get`  -  Get
- `halopsa-cli email-template-variable list`  -  List

**eracent**  -  Manage eracent

- `halopsa-cli eracent`  -  List

**eracent-details**  -  Manage eracent details

- `halopsa-cli eracent-details create`  -  Create
- `halopsa-cli eracent-details delete`  -  Delete
- `halopsa-cli eracent-details get`  -  Get
- `halopsa-cli eracent-details list`  -  List

**event**  -  Manage event

- `halopsa-cli event create`  -  Create
- `halopsa-cli event delete`  -  Delete
- `halopsa-cli event get`  -  Get
- `halopsa-cli event list`  -  List

**event-rule**  -  Manage event rule

- `halopsa-cli event-rule create`  -  Create
- `halopsa-cli event-rule delete`  -  Delete
- `halopsa-cli event-rule get`  -  Get
- `halopsa-cli event-rule list`  -  List

**exact-details**  -  Manage exact details

- `halopsa-cli exact-details create`  -  Create
- `halopsa-cli exact-details delete`  -  Delete
- `halopsa-cli exact-details get`  -  Use this to return a single instance of ExactDetails. 				Requires authentication.
- `halopsa-cli exact-details list`  -  Use this to return multiple ExactDetails. 				Requires authentication.

**example**  -  Manage example

- `halopsa-cli example`  -  List

**expense**  -  Manage expense

- `halopsa-cli expense create`  -  Create
- `halopsa-cli expense list`  -  List

**external-chat-message**  -  Manage external chat message

- `halopsa-cli external-chat-message create`  -  Create
- `halopsa-cli external-chat-message delete`  -  Delete
- `halopsa-cli external-chat-message get`  -  Get
- `halopsa-cli external-chat-message list`  -  List

**external-link**  -  Manage external link

- `halopsa-cli external-link create`  -  Create
- `halopsa-cli external-link create-externallink`  -  Create externallink
- `halopsa-cli external-link delete`  -  Delete
- `halopsa-cli external-link get`  -  Use this to return a single instance of ExternalLink. 				Requires authentication.
- `halopsa-cli external-link list`  -  Use this to return multiple ExternalLink. 				Requires authentication.

**facebook-details**  -  Manage facebook details

- `halopsa-cli facebook-details create`  -  Create
- `halopsa-cli facebook-details delete`  -  Delete
- `halopsa-cli facebook-details get`  -  Use this to return a single instance of FacebookDetails. 				Requires authentication.
- `halopsa-cli facebook-details list`  -  Use this to return multiple FacebookDetails. 				Requires authentication.

**faqlists**  -  Manage faqlists

- `halopsa-cli faqlists create`  -  Create
- `halopsa-cli faqlists delete`  -  Delete
- `halopsa-cli faqlists get`  -  Use this to return a single instance of FAQListHead. 				Requires authentication.
- `halopsa-cli faqlists list`  -  Use this to return multiple FAQListHead. 				Requires authentication.

**fault-view-log**  -  Manage fault view log

- `halopsa-cli fault-view-log`  -  List

**faults-forecasting**  -  Manage faults forecasting

- `halopsa-cli faults-forecasting create`  -  Create
- `halopsa-cli faults-forecasting get`  -  Use this to return a single instance of FaultsForecasting. 				Requires authentication.

**features**  -  Manage features

- `halopsa-cli features create`  -  Create
- `halopsa-cli features get`  -  Use this to return a single instance of ModuleSetup. 				Requires authentication.
- `halopsa-cli features list`  -  Use this to return multiple ModuleSetup. 				Requires authentication.

**feed**  -  Manage feed

- `halopsa-cli feed`  -  Use this to return multiple Feed. 				Requires authentication.

**feedback_items**  -  Manage feedback items

- `halopsa-cli feedback-items create`  -  Create
- `halopsa-cli feedback-items delete`  -  Delete
- `halopsa-cli feedback-items get`  -  Use this to return a single instance of Feedback. 				Requires authentication.
- `halopsa-cli feedback-items list`  -  List
- `halopsa-cli feedback-items list-feedback`  -  List feedback

**field**  -  Manage field

- `halopsa-cli field create`  -  Create
- `halopsa-cli field create-addfieldtoall`  -  Create addfieldtoall
- `halopsa-cli field delete`  -  Delete specific Field. 				Requires authentication.
- `halopsa-cli field get`  -  Use this to return a single instance of Field. 				Requires authentication.
- `halopsa-cli field list`  -  Use this to return multiple Field. 				Requires authentication.

**field-group**  -  Manage field group

- `halopsa-cli field-group create`  -  Create
- `halopsa-cli field-group delete`  -  Delete
- `halopsa-cli field-group get`  -  Use this to return a single instance of FieldGroup. 				Requires authentication.
- `halopsa-cli field-group list`  -  Use this to return multiple FieldGroup. 				Requires authentication.

**field-info**  -  Manage field info

- `halopsa-cli field-info create`  -  Create
- `halopsa-cli field-info delete`  -  Delete
- `halopsa-cli field-info get`  -  Use this to return a single instance of FieldInfo. 				Requires authentication.
- `halopsa-cli field-info list`  -  Use this to return multiple FieldInfo. 				Requires authentication.

**forecast-details**  -  Manage forecast details

- `halopsa-cli forecast-details create`  -  Create
- `halopsa-cli forecast-details delete`  -  Delete
- `halopsa-cli forecast-details get`  -  Get
- `halopsa-cli forecast-details list`  -  List

**forethought-details**  -  Manage forethought details

- `halopsa-cli forethought-details create`  -  Create
- `halopsa-cli forethought-details delete`  -  Delete
- `halopsa-cli forethought-details get`  -  Get
- `halopsa-cli forethought-details list`  -  List

**formattedemail**  -  Manage formattedemail

- `halopsa-cli formattedemail create`  -  Create
- `halopsa-cli formattedemail delete`  -  Delete
- `halopsa-cli formattedemail get`  -  Use this to return a single instance of formattedemail. 				Requires authentication.
- `halopsa-cli formattedemail list`  -  List

**fortnox-details**  -  Manage fortnox details

- `halopsa-cli fortnox-details create`  -  Create
- `halopsa-cli fortnox-details delete`  -  Delete
- `halopsa-cli fortnox-details get`  -  Get
- `halopsa-cli fortnox-details list`  -  List

**go-to-resolve**  -  Manage go to resolve

- `halopsa-cli go-to-resolve list`  -  List
- `halopsa-cli go-to-resolve list-gotoresolve`  -  List gotoresolve

**google-business-details**  -  Manage google business details

- `halopsa-cli google-business-details create`  -  Create
- `halopsa-cli google-business-details delete`  -  Delete
- `halopsa-cli google-business-details get`  -  Get
- `halopsa-cli google-business-details list`  -  List

**gworkspace-details**  -  Manage gworkspace details

- `halopsa-cli gworkspace-details create`  -  Create
- `halopsa-cli gworkspace-details delete`  -  Delete
- `halopsa-cli gworkspace-details get`  -  Get
- `halopsa-cli gworkspace-details list`  -  List

**halo-device-info**  -  Manage halo device info

- `halopsa-cli halo-device-info create`  -  Create
- `halopsa-cli halo-device-info delete`  -  Delete
- `halopsa-cli halo-device-info get`  -  Get

**halo-field**  -  Manage halo field

- `halopsa-cli halo-field`  -  List

**halo-integration**  -  Manage halo integration

- `halopsa-cli halo-integration create`  -  Create
- `halopsa-cli halo-integration create-halointegration`  -  Create halointegration
- `halopsa-cli halo-integration list`  -  List

**halo-news**  -  Manage halo news

- `halopsa-cli halo-news create`  -  Create
- `halopsa-cli halo-news create-halonews`  -  Create halonews
- `halopsa-cli halo-news delete`  -  Delete
- `halopsa-cli halo-news get`  -  Use this to return a single instance of HaloNews. 				Requires authentication.
- `halopsa-cli halo-news list`  -  List

**halo_search**  -  Manage halo search

- `halopsa-cli halo-search`  -  Use this to return multiple Search. 				Requires authentication.

**health**  -  Manage health

- `halopsa-cli health list`  -  List
- `halopsa-cli health list-hashing`  -  List hashing

**historical-ticket-volumes**  -  Manage historical ticket volumes

- `halopsa-cli historical-ticket-volumes create`  -  Create
- `halopsa-cli historical-ticket-volumes delete`  -  Delete
- `halopsa-cli historical-ticket-volumes get`  -  Get
- `halopsa-cli historical-ticket-volumes list`  -  List

**holiday**  -  Manage holiday

- `halopsa-cli holiday create`  -  Create
- `halopsa-cli holiday delete`  -  Delete
- `halopsa-cli holiday get`  -  Use this to return a single instance of Holidays. 				Requires authentication.
- `halopsa-cli holiday list`  -  Use this to return multiple Holidays. 				Requires authentication.

**hopewiser**  -  Manage hopewiser

- `halopsa-cli hopewiser`  -  List

**impersonation-request**  -  Manage impersonation request

- `halopsa-cli impersonation-request`  -  Create

**import-csv**  -  Manage import csv

- `halopsa-cli import-csv create`  -  Create
- `halopsa-cli import-csv delete`  -  Delete
- `halopsa-cli import-csv get`  -  Use this to return a single instance of ImportCsv. 				Requires authentication.
- `halopsa-cli import-csv list`  -  Use this to return multiple ImportCsv. 				Requires authentication.

**incoming-event**  -  Manage incoming event

- `halopsa-cli incoming-event create`  -  Create
- `halopsa-cli incoming-event create-incomingevent`  -  Create incomingevent
- `halopsa-cli incoming-event delete`  -  Delete
- `halopsa-cli incoming-event get`  -  Get
- `halopsa-cli incoming-event list`  -  List

**incoming-webhook**  -  Manage incoming webhook

- `halopsa-cli incoming-webhook create`  -  Create
- `halopsa-cli incoming-webhook create-incomingwebhook`  -  Create incomingwebhook
- `halopsa-cli incoming-webhook delete`  -  Delete
- `halopsa-cli incoming-webhook get`  -  Get
- `halopsa-cli incoming-webhook list`  -  List

**incoming-webhook-attempt**  -  Manage incoming webhook attempt

- `halopsa-cli incoming-webhook-attempt`  -  List

**incomingemail**  -  Manage incomingemail

- `halopsa-cli incomingemail create`  -  Create
- `halopsa-cli incomingemail create-addtoticket`  -  Create addtoticket
- `halopsa-cli incomingemail delete`  -  Delete
- `halopsa-cli incomingemail get`  -  Use this to return a single instance of IncomingEmail. 				Requires authentication.
- `halopsa-cli incomingemail list`  -  Use this to return multiple IncomingEmail. 				Requires authentication.

**ingram-micro-details**  -  Manage ingram micro details

- `halopsa-cli ingram-micro-details create`  -  Create
- `halopsa-cli ingram-micro-details delete`  -  Delete
- `halopsa-cli ingram-micro-details get`  -  Use this to return a single instance of IngramMicroDetails. 				Requires authentication.
- `halopsa-cli ingram-micro-details list`  -  List

**ingram-micro-reseller**  -  Manage ingram micro reseller

- `halopsa-cli ingram-micro-reseller list`  -  List
- `halopsa-cli ingram-micro-reseller list-ingrammicroreseller`  -  List ingrammicroreseller

**ingram-micro-reseller-details**  -  Manage ingram micro reseller details

- `halopsa-cli ingram-micro-reseller-details create`  -  Create
- `halopsa-cli ingram-micro-reseller-details delete`  -  Delete
- `halopsa-cli ingram-micro-reseller-details get`  -  Get
- `halopsa-cli ingram-micro-reseller-details list`  -  List

**instance**  -  Manage instance

- `halopsa-cli instance create`  -  Create
- `halopsa-cli instance get`  -  Get
- `halopsa-cli instance list`  -  Use this to return multiple Instance. 				Requires authentication.

**instance-info**  -  Manage instance info

- `halopsa-cli instance-info`  -  List

**integration-configuration**  -  Manage integration configuration

- `halopsa-cli integration-configuration create`  -  Create
- `halopsa-cli integration-configuration get`  -  Use this to return a single instance of IntegrationConfiguration. 				Requires authentication.
- `halopsa-cli integration-configuration list`  -  List

**integration-data**  -  Manage integration data

- `halopsa-cli integration-data create`  -  Create
- `halopsa-cli integration-data create-integrationdata`  -  Create integrationdata
- `halopsa-cli integration-data create-integrationdata-10`  -  Create integrationdata 10
- `halopsa-cli integration-data create-integrationdata-11`  -  . 				Requires authentication.
- `halopsa-cli integration-data create-integrationdata-12`  -  Create integrationdata 12
- `halopsa-cli integration-data create-integrationdata-13`  -  Create integrationdata 13
- `halopsa-cli integration-data create-integrationdata-14`  -  Create integrationdata 14
- `halopsa-cli integration-data create-integrationdata-15`  -  Create integrationdata 15
- `halopsa-cli integration-data create-integrationdata-16`  -  Create integrationdata 16
- `halopsa-cli integration-data create-integrationdata-17`  -  Create integrationdata 17
- `halopsa-cli integration-data create-integrationdata-18`  -  Create integrationdata 18
- `halopsa-cli integration-data create-integrationdata-19`  -  Create integrationdata 19
- `halopsa-cli integration-data create-integrationdata-2`  -  Create integrationdata 2
- `halopsa-cli integration-data create-integrationdata-20`  -  Create integrationdata 20
- `halopsa-cli integration-data create-integrationdata-21`  -  Create integrationdata 21
- `halopsa-cli integration-data create-integrationdata-22`  -  Create integrationdata 22
- `halopsa-cli integration-data create-integrationdata-23`  -  Create integrationdata 23
- `halopsa-cli integration-data create-integrationdata-24`  -  Create integrationdata 24
- `halopsa-cli integration-data create-integrationdata-25`  -  Create integrationdata 25
- `halopsa-cli integration-data create-integrationdata-26`  -  Create integrationdata 26
- `halopsa-cli integration-data create-integrationdata-27`  -  Create integrationdata 27
- `halopsa-cli integration-data create-integrationdata-28`  -  Create integrationdata 28
- `halopsa-cli integration-data create-integrationdata-29`  -  Create integrationdata 29
- `halopsa-cli integration-data create-integrationdata-3`  -  Create integrationdata 3
- `halopsa-cli integration-data create-integrationdata-30`  -  Create integrationdata 30
- `halopsa-cli integration-data create-integrationdata-31`  -  Create integrationdata 31
- `halopsa-cli integration-data create-integrationdata-32`  -  Create integrationdata 32
- `halopsa-cli integration-data create-integrationdata-33`  -  Create integrationdata 33
- `halopsa-cli integration-data create-integrationdata-34`  -  Create integrationdata 34
- `halopsa-cli integration-data create-integrationdata-35`  -  Create integrationdata 35
- `halopsa-cli integration-data create-integrationdata-36`  -  Create integrationdata 36
- `halopsa-cli integration-data create-integrationdata-37`  -  Create integrationdata 37
- `halopsa-cli integration-data create-integrationdata-38`  -  Create integrationdata 38
- `halopsa-cli integration-data create-integrationdata-39`  -  Create integrationdata 39
- `halopsa-cli integration-data create-integrationdata-4`  -  Create integrationdata 4
- `halopsa-cli integration-data create-integrationdata-40`  -  Create integrationdata 40
- `halopsa-cli integration-data create-integrationdata-41`  -  Create integrationdata 41
- `halopsa-cli integration-data create-integrationdata-42`  -  Create integrationdata 42
- `halopsa-cli integration-data create-integrationdata-5`  -  Create integrationdata 5
- `halopsa-cli integration-data create-integrationdata-6`  -  Create integrationdata 6
- `halopsa-cli integration-data create-integrationdata-7`  -  Create integrationdata 7
- `halopsa-cli integration-data create-integrationdata-8`  -  Create integrationdata 8
- `halopsa-cli integration-data create-integrationdata-9`  -  Create integrationdata 9
- `halopsa-cli integration-data get`  -  Get
- `halopsa-cli integration-data list`  -  List
- `halopsa-cli integration-data list-integrationdata`  -  List integrationdata
- `halopsa-cli integration-data list-integrationdata-10`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-100`  -  List integrationdata 100
- `halopsa-cli integration-data list-integrationdata-101`  -  List integrationdata 101
- `halopsa-cli integration-data list-integrationdata-102`  -  List integrationdata 102
- `halopsa-cli integration-data list-integrationdata-103`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-104`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-105`  -  List integrationdata 105
- `halopsa-cli integration-data list-integrationdata-106`  -  List integrationdata 106
- `halopsa-cli integration-data list-integrationdata-107`  -  List integrationdata 107
- `halopsa-cli integration-data list-integrationdata-108`  -  List integrationdata 108
- `halopsa-cli integration-data list-integrationdata-109`  -  List integrationdata 109
- `halopsa-cli integration-data list-integrationdata-11`  -  List integrationdata 11
- `halopsa-cli integration-data list-integrationdata-110`  -  List integrationdata 110
- `halopsa-cli integration-data list-integrationdata-111`  -  List integrationdata 111
- `halopsa-cli integration-data list-integrationdata-112`  -  List integrationdata 112
- `halopsa-cli integration-data list-integrationdata-12`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-13`  -  List integrationdata 13
- `halopsa-cli integration-data list-integrationdata-14`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-15`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-16`  -  List integrationdata 16
- `halopsa-cli integration-data list-integrationdata-17`  -  List integrationdata 17
- `halopsa-cli integration-data list-integrationdata-18`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-19`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-2`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-20`  -  List integrationdata 20
- `halopsa-cli integration-data list-integrationdata-21`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-22`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-23`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-24`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-25`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-26`  -  List integrationdata 26
- `halopsa-cli integration-data list-integrationdata-27`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-28`  -  List integrationdata 28
- `halopsa-cli integration-data list-integrationdata-29`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-3`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-30`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-31`  -  List integrationdata 31
- `halopsa-cli integration-data list-integrationdata-32`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-33`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-34`  -  List integrationdata 34
- `halopsa-cli integration-data list-integrationdata-35`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-36`  -  List integrationdata 36
- `halopsa-cli integration-data list-integrationdata-37`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-38`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-39`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-4`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-40`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-41`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-42`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-43`  -  List integrationdata 43
- `halopsa-cli integration-data list-integrationdata-44`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-45`  -  List integrationdata 45
- `halopsa-cli integration-data list-integrationdata-46`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-47`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-48`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-49`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-5`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-50`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-51`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-52`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-53`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-54`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-55`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-56`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-57`  -  List integrationdata 57
- `halopsa-cli integration-data list-integrationdata-58`  -  List integrationdata 58
- `halopsa-cli integration-data list-integrationdata-59`  -  List integrationdata 59
- `halopsa-cli integration-data list-integrationdata-6`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-60`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-61`  -  List integrationdata 61
- `halopsa-cli integration-data list-integrationdata-62`  -  List integrationdata 62
- `halopsa-cli integration-data list-integrationdata-63`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-64`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-65`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-66`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-67`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-68`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-69`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-7`  -  List integrationdata 7
- `halopsa-cli integration-data list-integrationdata-70`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-71`  -  List integrationdata 71
- `halopsa-cli integration-data list-integrationdata-72`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-73`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-74`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-75`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-76`  -  List integrationdata 76
- `halopsa-cli integration-data list-integrationdata-77`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-78`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-79`  -  List integrationdata 79
- `halopsa-cli integration-data list-integrationdata-8`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-80`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-81`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-82`  -  List integrationdata 82
- `halopsa-cli integration-data list-integrationdata-83`  -  List integrationdata 83
- `halopsa-cli integration-data list-integrationdata-84`  -  List integrationdata 84
- `halopsa-cli integration-data list-integrationdata-85`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-86`  -  List integrationdata 86
- `halopsa-cli integration-data list-integrationdata-87`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-88`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-89`  -  List integrationdata 89
- `halopsa-cli integration-data list-integrationdata-9`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-90`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-91`  -  List integrationdata 91
- `halopsa-cli integration-data list-integrationdata-92`  -  List integrationdata 92
- `halopsa-cli integration-data list-integrationdata-93`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-94`  -  List integrationdata 94
- `halopsa-cli integration-data list-integrationdata-95`  -  List integrationdata 95
- `halopsa-cli integration-data list-integrationdata-96`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-97`  -  . 				Requires authentication.
- `halopsa-cli integration-data list-integrationdata-98`  -  List integrationdata 98
- `halopsa-cli integration-data list-integrationdata-99`  -  List integrationdata 99

**integration-delta**  -  Manage integration delta

- `halopsa-cli integration-delta create`  -  Create
- `halopsa-cli integration-delta delete`  -  Delete
- `halopsa-cli integration-delta get`  -  Get
- `halopsa-cli integration-delta list`  -  List

**integration-error**  -  Manage integration error

- `halopsa-cli integration-error create`  -  Create
- `halopsa-cli integration-error delete`  -  Delete
- `halopsa-cli integration-error get`  -  Use this to return a single instance of IntegrationError. 				Requires authentication.
- `halopsa-cli integration-error list`  -  Use this to return multiple IntegrationError. 				Requires authentication.

**integration-export**  -  Manage integration export

- `halopsa-cli integration-export create`  -  Create
- `halopsa-cli integration-export delete`  -  Delete
- `halopsa-cli integration-export list`  -  Use this to return multiple IntegrationExport. 				Requires authentication.

**integration-field-data**  -  Manage integration field data

- `halopsa-cli integration-field-data create`  -  Create
- `halopsa-cli integration-field-data delete`  -  Delete
- `halopsa-cli integration-field-data get`  -  Get
- `halopsa-cli integration-field-data list`  -  List

**integration-field-mapping**  -  Manage integration field mapping

- `halopsa-cli integration-field-mapping`  -  Use this to return multiple IntegrationFieldMapping. 				Requires authentication.

**integration-look-up**  -  Manage integration look up

- `halopsa-cli integration-look-up create`  -  Create
- `halopsa-cli integration-look-up list`  -  List

**integration-request**  -  Manage integration request

- `halopsa-cli integration-request create`  -  Create
- `halopsa-cli integration-request delete`  -  Delete
- `halopsa-cli integration-request get`  -  Use this to return a single instance of IntegrationRequest. 				Requires authentication.
- `halopsa-cli integration-request list`  -  Use this to return multiple IntegrationRequest. 				Requires authentication.

**integration-runbook-variable-group**  -  Manage integration runbook variable group

- `halopsa-cli integration-runbook-variable-group get`  -  Use this to return a single instance of IntegrationRunbookVariableGroup. 				Requires authentication.
- `halopsa-cli integration-runbook-variable-group list`  -  Use this to return multiple IntegrationRunbookVariableGroup. 				Requires authentication.

**integration-site-mapping**  -  Manage integration site mapping

- `halopsa-cli integration-site-mapping`  -  Use this to return multiple IntegrationSiteMapping. 				Requires authentication.

**integrator-log**  -  Manage integrator log

- `halopsa-cli integrator-log`  -  Use this to return multiple IntegratorLog. 				Requires authentication.

**integrator-schedule**  -  Manage integrator schedule

- `halopsa-cli integrator-schedule`  -  Use this to return multiple IntegratorSchedule. 				Requires authentication.

**integrator-trace**  -  Manage integrator trace

- `halopsa-cli integrator-trace get`  -  Get
- `halopsa-cli integrator-trace list`  -  List

**invoice**  -  Manage invoice

- `halopsa-cli invoice create`  -  Create
- `halopsa-cli invoice create-pdf`  -  Create pdf
- `halopsa-cli invoice create-updatelines`  -  Create updatelines
- `halopsa-cli invoice create-view`  -  Create view
- `halopsa-cli invoice delete`  -  Delete specific InvoiceHeader. 				Requires authentication.
- `halopsa-cli invoice get`  -  Use this to return a single instance of InvoiceHeader. 				Requires authentication.
- `halopsa-cli invoice list`  -  Use this to return multiple InvoiceHeader. 				Requires authentication.
- `halopsa-cli invoice list-lines`  -  List lines

**invoice-change**  -  Manage invoice change

- `halopsa-cli invoice-change create`  -  Create
- `halopsa-cli invoice-change list`  -  Use this to return multiple InvoiceChange. 				Requires authentication.

**invoice-detail-pro-rata**  -  Manage invoice detail pro rata

- `halopsa-cli invoice-detail-pro-rata`  -  List

**invoice-payment**  -  Manage invoice payment

- `halopsa-cli invoice-payment create`  -  Create
- `halopsa-cli invoice-payment delete`  -  Delete
- `halopsa-cli invoice-payment get`  -  Use this to return a single instance of InvoicePayment. 				Requires authentication.
- `halopsa-cli invoice-payment list`  -  Use this to return multiple InvoicePayment. 				Requires authentication.

**islonline**  -  Manage islonline

- `halopsa-cli islonline create`  -  Create
- `halopsa-cli islonline list`  -  List

**item**  -  Manage item

- `halopsa-cli item create`  -  Create
- `halopsa-cli item create-newaccountsid`  -  Create newaccountsid
- `halopsa-cli item delete`  -  Delete
- `halopsa-cli item get`  -  Use this to return a single instance of Item. 				Requires authentication.
- `halopsa-cli item list`  -  Use this to return multiple Item. 				Requires authentication.

**item-accounts-link**  -  Manage item accounts link

- `halopsa-cli item-accounts-link create`  -  Create
- `halopsa-cli item-accounts-link create-itemaccountslink`  -  Create itemaccountslink
- `halopsa-cli item-accounts-link delete`  -  Delete
- `halopsa-cli item-accounts-link get`  -  Get
- `halopsa-cli item-accounts-link list`  -  List

**item-group**  -  Manage item group

- `halopsa-cli item-group create`  -  Create
- `halopsa-cli item-group delete`  -  Delete
- `halopsa-cli item-group get`  -  Use this to return a single instance of ItemGroup. 				Requires authentication.
- `halopsa-cli item-group list`  -  List

**item-stock**  -  Manage item stock

- `halopsa-cli item-stock create`  -  Create
- `halopsa-cli item-stock delete`  -  Delete
- `halopsa-cli item-stock get`  -  Use this to return a single instance of ItemStock. 				Requires authentication.
- `halopsa-cli item-stock list`  -  Use this to return multiple ItemStock. 				Requires authentication.

**item-stock-history**  -  Manage item stock history

- `halopsa-cli item-stock-history get`  -  Get
- `halopsa-cli item-stock-history list`  -  Use this to return multiple ItemStockHistory. 				Requires authentication.

**itemsupplier**  -  Manage itemsupplier

- `halopsa-cli itemsupplier create`  -  Create
- `halopsa-cli itemsupplier delete`  -  Delete
- `halopsa-cli itemsupplier get`  -  Use this to return a single instance of ItemSupplier. 				Requires authentication.
- `halopsa-cli itemsupplier list`  -  List

**jamf-details**  -  Manage jamf details

- `halopsa-cli jamf-details create`  -  Create
- `halopsa-cli jamf-details delete`  -  Delete
- `halopsa-cli jamf-details get`  -  Get
- `halopsa-cli jamf-details list`  -  List

**jira-details**  -  Manage jira details

- `halopsa-cli jira-details create`  -  Create
- `halopsa-cli jira-details delete`  -  Delete
- `halopsa-cli jira-details get`  -  Get
- `halopsa-cli jira-details list`  -  List

**journey**  -  Manage journey

- `halopsa-cli journey create`  -  Create
- `halopsa-cli journey delete`  -  Delete
- `halopsa-cli journey get`  -  Use this to return a single instance of Journey. 				Requires authentication.
- `halopsa-cli journey list`  -  List

**kandji**  -  Manage kandji

- `halopsa-cli kandji`  -  List

**kandji-details**  -  Manage kandji details

- `halopsa-cli kandji-details create`  -  Create
- `halopsa-cli kandji-details delete`  -  Delete
- `halopsa-cli kandji-details get`  -  Get
- `halopsa-cli kandji-details list`  -  List

**kaseya-vsax**  -  Manage kaseya vsax

- `halopsa-cli kaseya-vsax create`  -  Create
- `halopsa-cli kaseya-vsax delete`  -  Delete
- `halopsa-cli kaseya-vsax list`  -  List

**kaseya-vsaxdetails**  -  Manage kaseya vsaxdetails

- `halopsa-cli kaseya-vsaxdetails create`  -  Create
- `halopsa-cli kaseya-vsaxdetails delete`  -  Delete
- `halopsa-cli kaseya-vsaxdetails get`  -  Get
- `halopsa-cli kaseya-vsaxdetails list`  -  List

**kashflow-details**  -  Manage kashflow details

- `halopsa-cli kashflow-details create`  -  Create
- `halopsa-cli kashflow-details delete`  -  Delete
- `halopsa-cli kashflow-details get`  -  Use this to return a single instance of KashflowDetails. 				Requires authentication.
- `halopsa-cli kashflow-details list`  -  Use this to return multiple KashflowDetails. 				Requires authentication.

**kbarticle**  -  Manage kbarticle

- `halopsa-cli kbarticle create`  -  Create
- `halopsa-cli kbarticle create-vote`  -  Create vote
- `halopsa-cli kbarticle delete`  -  Delete
- `halopsa-cli kbarticle get`  -  Use this to return a single instance of KBEntry. 				Requires authentication.
- `halopsa-cli kbarticle list`  -  Use this to return multiple KBEntry. 				Requires authentication.

**kbarticle-anon**  -  Manage kbarticle anon

- `halopsa-cli kbarticle-anon get`  -  Get
- `halopsa-cli kbarticle-anon list`  -  List

**key-vault**  -  Manage key vault

- `halopsa-cli key-vault create`  -  Create
- `halopsa-cli key-vault delete`  -  Delete
- `halopsa-cli key-vault get`  -  Get
- `halopsa-cli key-vault list`  -  List

**languages**  -  Manage languages

- `halopsa-cli languages create`  -  Create
- `halopsa-cli languages delete`  -  Delete
- `halopsa-cli languages get`  -  Use this to return a single instance of LanguagePack. 				Requires authentication.
- `halopsa-cli languages list`  -  Use this to return multiple LanguagePack. 				Requires authentication.

**lap-safe**  -  Manage lap safe

- `halopsa-cli lap-safe list`  -  List
- `halopsa-cli lap-safe list-lapsafe`  -  List lapsafe
- `halopsa-cli lap-safe list-lapsafe-2`  -  List lapsafe 2

**ldapconnection**  -  Manage ldapconnection

- `halopsa-cli ldapconnection create`  -  Create
- `halopsa-cli ldapconnection delete`  -  Delete
- `halopsa-cli ldapconnection get`  -  Use this to return a single instance of LDAPConnection. 				Requires authentication.
- `halopsa-cli ldapconnection list`  -  Use this to return multiple LDAPConnection. 				Requires authentication.

**licence-change**  -  Manage licence change

- `halopsa-cli licence-change`  -  Use this to return multiple LicenceChange. 				Requires authentication.

**license-info**  -  Manage license info

- `halopsa-cli license-info create`  -  Create
- `halopsa-cli license-info list`  -  Use this to return multiple LicenceInfo. 				Requires authentication.
- `halopsa-cli license-info list-licenseinfo`  -  List licenseinfo

**login-token**  -  Manage login token

- `halopsa-cli login-token`  -  Create

**lookup**  -  Manage lookup

- `halopsa-cli lookup create`  -  Create
- `halopsa-cli lookup create-clearcache`  -  Create clearcache
- `halopsa-cli lookup delete`  -  Delete
- `halopsa-cli lookup get`  -  Use this to return a single instance of Lookup. 				Requires authentication.
- `halopsa-cli lookup list`  -  Use this to return multiple Lookup. 				Requires authentication.

**mail**  -  Manage mail

- `halopsa-cli mail create`  -  Create
- `halopsa-cli mail create-integrator`  -  Create integrator
- `halopsa-cli mail create-integrator-2`  -  Create integrator 2
- `halopsa-cli mail create-integrator-3`  -  Create integrator 3
- `halopsa-cli mail create-integrator-4`  -  Create integrator 4
- `halopsa-cli mail create-processmail`  -  Create processmail

**mail-campaign**  -  Manage mail campaign

- `halopsa-cli mail-campaign create`  -  Create
- `halopsa-cli mail-campaign delete`  -  Delete
- `halopsa-cli mail-campaign get`  -  Get
- `halopsa-cli mail-campaign list`  -  List

**mail-campaign-email**  -  Manage mail campaign email

- `halopsa-cli mail-campaign-email create`  -  Create
- `halopsa-cli mail-campaign-email delete`  -  Delete
- `halopsa-cli mail-campaign-email get`  -  Get
- `halopsa-cli mail-campaign-email list`  -  List

**mail-campaign-log**  -  Manage mail campaign log

- `halopsa-cli mail-campaign-log get`  -  Get
- `halopsa-cli mail-campaign-log list`  -  List

**mailbox**  -  Manage mailbox

- `halopsa-cli mailbox create`  -  Create
- `halopsa-cli mailbox delete`  -  Delete
- `halopsa-cli mailbox get`  -  Use this to return a single instance of Mailbox. 				Requires authentication.
- `halopsa-cli mailbox list`  -  Use this to return multiple Mailbox. 				Requires authentication.

**mailbox-credential**  -  Manage mailbox credential

- `halopsa-cli mailbox-credential create`  -  Create
- `halopsa-cli mailbox-credential delete`  -  Delete
- `halopsa-cli mailbox-credential get`  -  Get
- `halopsa-cli mailbox-credential list`  -  List

**mailchimp**  -  Manage mailchimp

- `halopsa-cli mailchimp`  -  List

**manage-engine**  -  Manage manage engine

- `halopsa-cli manage-engine`  -  List

**manage-engine-details**  -  Manage manage engine details

- `halopsa-cli manage-engine-details create`  -  Create
- `halopsa-cli manage-engine-details delete`  -  Delete
- `halopsa-cli manage-engine-details get`  -  Get
- `halopsa-cli manage-engine-details list`  -  List

**marketing-unsubscribe**  -  Manage marketing unsubscribe

- `halopsa-cli marketing-unsubscribe create`  -  Create
- `halopsa-cli marketing-unsubscribe delete`  -  Delete
- `halopsa-cli marketing-unsubscribe get`  -  Get
- `halopsa-cli marketing-unsubscribe list`  -  List

**mattermost-channel-details**  -  Manage mattermost channel details

- `halopsa-cli mattermost-channel-details`  -  List

**mattermost-details**  -  Manage mattermost details

- `halopsa-cli mattermost-details create`  -  Create
- `halopsa-cli mattermost-details delete`  -  Delete
- `halopsa-cli mattermost-details get`  -  Get
- `halopsa-cli mattermost-details list`  -  List

**mcp**  -  Manage mcp

- `halopsa-cli mcp create`  -  Create
- `halopsa-cli mcp delete`  -  Delete
- `halopsa-cli mcp list`  -  List

**meter-reading**  -  Manage meter reading

- `halopsa-cli meter-reading create`  -  Create
- `halopsa-cli meter-reading get`  -  Use this to return a single instance of DeviceMeterReading. 				Requires authentication.
- `halopsa-cli meter-reading list`  -  Use this to return multiple DeviceMeterReading. 				Requires authentication.

**microsoft-subscription-mapping**  -  Manage microsoft subscription mapping

- `halopsa-cli microsoft-subscription-mapping create`  -  Create
- `halopsa-cli microsoft-subscription-mapping delete`  -  Delete
- `halopsa-cli microsoft-subscription-mapping get`  -  Get
- `halopsa-cli microsoft-subscription-mapping list`  -  List

**microsoft-teams**  -  Manage microsoft teams

- `halopsa-cli microsoft-teams`  -  List

**microsoft-teams-mapping**  -  Manage microsoft teams mapping

- `halopsa-cli microsoft-teams-mapping create`  -  Create
- `halopsa-cli microsoft-teams-mapping delete`  -  Delete
- `halopsa-cli microsoft-teams-mapping get`  -  Get
- `halopsa-cli microsoft-teams-mapping list`  -  List

**mo**  -  Manage mo

- `halopsa-cli mo create`  -  Create
- `halopsa-cli mo delete`  -  Delete
- `halopsa-cli mo get`  -  Get
- `halopsa-cli mo list`  -  List
- `halopsa-cli mo list-b`  -  List b
- `halopsa-cli mo list-r`  -  List r

**myobdetails**  -  Manage myobdetails

- `halopsa-cli myobdetails create`  -  Create
- `halopsa-cli myobdetails delete`  -  Delete
- `halopsa-cli myobdetails get`  -  Get
- `halopsa-cli myobdetails list`  -  List

**ncentral-details**  -  Manage ncentral details

- `halopsa-cli ncentral-details create`  -  Create
- `halopsa-cli ncentral-details delete`  -  Delete
- `halopsa-cli ncentral-details get`  -  Use this to return a single instance of NCentralDetails. 				Requires authentication.
- `halopsa-cli ncentral-details list`  -  Use this to return multiple NCentralDetails. 				Requires authentication.

**nhserverconfig**  -  Manage nhserverconfig

- `halopsa-cli nhserverconfig create`  -  Create
- `halopsa-cli nhserverconfig delete`  -  Delete
- `halopsa-cli nhserverconfig get`  -  Use this to return a single instance of NHServerConfig. 				Requires authentication.
- `halopsa-cli nhserverconfig list`  -  List

**notification**  -  Manage notification

- `halopsa-cli notification create`  -  Create
- `halopsa-cli notification delete`  -  Delete
- `halopsa-cli notification get`  -  Use this to return a single instance of UnameNotification. 				Requires authentication.
- `halopsa-cli notification list`  -  Use this to return multiple UnameNotification. 				Requires authentication.

**notification-log**  -  Manage notification log

- `halopsa-cli notification-log`  -  List

**notification-message**  -  Manage notification message

- `halopsa-cli notification-message create`  -  Create
- `halopsa-cli notification-message delete`  -  Delete
- `halopsa-cli notification-message get`  -  Use this to return a single instance of NotificationContent. 				Requires authentication.
- `halopsa-cli notification-message list`  -  List

**notifications**  -  Manage notifications

- `halopsa-cli notifications create`  -  Create
- `halopsa-cli notifications create-process`  -  Create process
- `halopsa-cli notifications delete`  -  Delete
- `halopsa-cli notifications get`  -  Use this to return a single instance of EscMsg. 				Requires authentication.
- `halopsa-cli notifications list`  -  Use this to return multiple EscMsg. 				Requires authentication.

**object-mapping-profile**  -  Manage object mapping profile

- `halopsa-cli object-mapping-profile`  -  List

**online-status**  -  Manage online status

- `halopsa-cli online-status create`  -  Create
- `halopsa-cli online-status list`  -  List

**opportunities**  -  Manage opportunities

- `halopsa-cli opportunities create`  -  Create
- `halopsa-cli opportunities create-view`  -  Create view
- `halopsa-cli opportunities delete`  -  Delete specific Faults. 				Requires authentication.
- `halopsa-cli opportunities get`  -  Use this to return a single instance of Faults. 				Requires authentication.
- `halopsa-cli opportunities list`  -  Use this to return multiple Faults. 				Requires authentication.

**order-line**  -  Manage order line

- `halopsa-cli order-line`  -  List

**organisation**  -  Manage organisation

- `halopsa-cli organisation create`  -  Create
- `halopsa-cli organisation delete`  -  Delete
- `halopsa-cli organisation get`  -  Use this to return a single instance of Organisation. 				Requires authentication.
- `halopsa-cli organisation list`  -  List

**outcome**  -  Manage outcome

- `halopsa-cli outcome create`  -  Create
- `halopsa-cli outcome delete`  -  Delete
- `halopsa-cli outcome get`  -  Use this to return a single instance of TOutcome. 				Requires authentication.
- `halopsa-cli outcome list`  -  Use this to return multiple TOutcome. 				Requires authentication.

**outgoing**  -  Manage outgoing

- `halopsa-cli outgoing create`  -  Create
- `halopsa-cli outgoing delete`  -  Delete
- `halopsa-cli outgoing get`  -  Use this to return a single instance of Outgoing. 				Requires authentication.
- `halopsa-cli outgoing list`  -  Use this to return multiple Outgoing. 				Requires authentication.

**outgoing-attempt**  -  Manage outgoing attempt

- `halopsa-cli outgoing-attempt get`  -  Use this to return a single instance of OutgoingAttempt. 				Requires authentication.
- `halopsa-cli outgoing-attempt list`  -  Use this to return multiple OutgoingAttempt. 				Requires authentication.

**outgoingemail**  -  Manage outgoingemail

- `halopsa-cli outgoingemail create`  -  Create
- `halopsa-cli outgoingemail delete`  -  Delete
- `halopsa-cli outgoingemail list`  -  Use this to return multiple Outgoingemail. 				Requires authentication.

**pagerdutymapping**  -  Manage pagerdutymapping

- `halopsa-cli pagerdutymapping`  -  Use this to return multiple PagerDutyMapping. 				Requires authentication.

**password-field**  -  Manage password field

- `halopsa-cli password-field create`  -  Create
- `halopsa-cli password-field get`  -  Use this to return a single instance of AuditPasswordField. 				Requires authentication.
- `halopsa-cli password-field list`  -  List

**pax8-details**  -  Manage pax8 details

- `halopsa-cli pax8-details create`  -  Create
- `halopsa-cli pax8-details delete`  -  Delete
- `halopsa-cli pax8-details get`  -  Get
- `halopsa-cli pax8-details list`  -  List

**pdf-template**  -  Manage pdf template

- `halopsa-cli pdf-template create`  -  Create
- `halopsa-cli pdf-template delete`  -  Delete
- `halopsa-cli pdf-template get`  -  Use this to return a single instance of PdfTemplate. 				Requires authentication.
- `halopsa-cli pdf-template list`  -  Use this to return multiple PdfTemplate. 				Requires authentication.

**pdf-template-repository**  -  Manage pdf template repository

- `halopsa-cli pdf-template-repository get`  -  Use this to return a single instance of PdfTemplate. 				Requires authentication.
- `halopsa-cli pdf-template-repository list`  -  Use this to return multiple PdfTemplate. 				Requires authentication.

**popup-note**  -  Manage popup note

- `halopsa-cli popup-note create`  -  Create
- `halopsa-cli popup-note list`  -  Use this to return multiple AreaPopup. 				Requires authentication.

**power-shell-script**  -  Manage power shell script

- `halopsa-cli power-shell-script create`  -  Create
- `halopsa-cli power-shell-script delete`  -  Delete
- `halopsa-cli power-shell-script get`  -  Use this to return a single instance of PowerShellScript. 				Requires authentication.
- `halopsa-cli power-shell-script list`  -  Use this to return multiple PowerShellScript. 				Requires authentication.

**power-shell-script-criteria**  -  Manage power shell script criteria

- `halopsa-cli power-shell-script-criteria create`  -  Create
- `halopsa-cli power-shell-script-criteria delete`  -  Delete
- `halopsa-cli power-shell-script-criteria get`  -  Use this to return a single instance of PowerShellScriptCriteria. 				Requires authentication.
- `halopsa-cli power-shell-script-criteria list`  -  Use this to return multiple PowerShellScriptCriteria. 				Requires authentication.

**power-shell-script-processing**  -  Manage power shell script processing

- `halopsa-cli power-shell-script-processing create`  -  Create
- `halopsa-cli power-shell-script-processing delete`  -  Delete
- `halopsa-cli power-shell-script-processing get`  -  Use this to return a single instance of PowerShellScriptProcessing. 				Requires authentication.
- `halopsa-cli power-shell-script-processing list`  -  Use this to return multiple PowerShellScriptProcessing. 				Requires authentication.

**priority**  -  Manage priority

- `halopsa-cli priority create`  -  Create
- `halopsa-cli priority delete`  -  Delete
- `halopsa-cli priority get`  -  Use this to return a single instance of Policy. 				Requires authentication.
- `halopsa-cli priority list`  -  Use this to return multiple Policy. 				Requires authentication.

**product**  -  Manage product

- `halopsa-cli product create`  -  Create
- `halopsa-cli product delete`  -  Delete
- `halopsa-cli product get`  -  Use this to return a single instance of ReleaseProduct. 				Requires authentication.
- `halopsa-cli product list`  -  Use this to return multiple ReleaseProduct. 				Requires authentication.

**product-branch**  -  Manage product branch

- `halopsa-cli product-branch`  -  Use this to return multiple ReleaseBranch. 				Requires authentication.

**product-component**  -  Manage product component

- `halopsa-cli product-component create`  -  Create
- `halopsa-cli product-component delete`  -  Delete
- `halopsa-cli product-component get`  -  Use this to return a single instance of ReleaseComponent. 				Requires authentication.
- `halopsa-cli product-component list`  -  Use this to return multiple ReleaseComponent. 				Requires authentication.

**project-setup-lines**  -  Manage project setup lines

- `halopsa-cli project-setup-lines`  -  Create

**projects**  -  Manage projects

- `halopsa-cli projects create`  -  Create
- `halopsa-cli projects create-view`  -  Create view
- `halopsa-cli projects delete`  -  Delete specific Faults. 				Requires authentication.
- `halopsa-cli projects get`  -  Use this to return a single instance of Faults. 				Requires authentication.
- `halopsa-cli projects list`  -  Use this to return multiple Faults. 				Requires authentication.

**prtgdetails**  -  Manage prtgdetails

- `halopsa-cli prtgdetails create`  -  Create
- `halopsa-cli prtgdetails delete`  -  Delete
- `halopsa-cli prtgdetails get`  -  Get
- `halopsa-cli prtgdetails list`  -  List

**publish-profiles**  -  Manage publish profiles

- `halopsa-cli publish-profiles create`  -  Create
- `halopsa-cli publish-profiles delete`  -  Delete
- `halopsa-cli publish-profiles get`  -  Get
- `halopsa-cli publish-profiles list`  -  List

**purchase-order**  -  Manage purchase order

- `halopsa-cli purchase-order create`  -  Create
- `halopsa-cli purchase-order create-purchaseorder`  -  Create purchaseorder
- `halopsa-cli purchase-order create-purchaseorder-2`  -  Create purchaseorder 2
- `halopsa-cli purchase-order delete`  -  Delete
- `halopsa-cli purchase-order get`  -  Use this to return a single instance of SupplierOrderHeader. 				Requires authentication.
- `halopsa-cli purchase-order list`  -  Use this to return multiple SupplierOrderHeader. 				Requires authentication.

**qualification**  -  Manage qualification

- `halopsa-cli qualification create`  -  Create
- `halopsa-cli qualification delete`  -  Delete
- `halopsa-cli qualification get`  -  Use this to return a single instance of Qualification. 				Requires authentication.
- `halopsa-cli qualification list`  -  Use this to return multiple Qualification. 				Requires authentication.

**quick-books-details**  -  Manage quick books details

- `halopsa-cli quick-books-details create`  -  Create
- `halopsa-cli quick-books-details delete`  -  Delete
- `halopsa-cli quick-books-details get`  -  Use this to return a single instance of QuickBooksDetails. 				Requires authentication.
- `halopsa-cli quick-books-details list`  -  Use this to return multiple QuickBooksDetails. 				Requires authentication.

**quotation**  -  Manage quotation

- `halopsa-cli quotation create`  -  Create
- `halopsa-cli quotation create-approval`  -  Create approval
- `halopsa-cli quotation create-lines`  -  Create lines
- `halopsa-cli quotation create-view`  -  Create view
- `halopsa-cli quotation delete`  -  Delete
- `halopsa-cli quotation get`  -  Use this to return a single instance of QuotationHeader. 				Requires authentication.
- `halopsa-cli quotation list`  -  Use this to return multiple QuotationHeader. 				Requires authentication.

**raynet**  -  Manage raynet

- `halopsa-cli raynet`  -  List

**raynet-details**  -  Manage raynet details

- `halopsa-cli raynet-details create`  -  Create
- `halopsa-cli raynet-details delete`  -  Delete
- `halopsa-cli raynet-details get`  -  Get
- `halopsa-cli raynet-details list`  -  List

**recurring-invoice**  -  Manage recurring invoice

- `halopsa-cli recurring-invoice create`  -  Create
- `halopsa-cli recurring-invoice create-recurringinvoice`  -  Create recurringinvoice
- `halopsa-cli recurring-invoice create-recurringinvoice-2`  -  Create recurringinvoice 2
- `halopsa-cli recurring-invoice create-recurringinvoice-3`  -  Create recurringinvoice 3
- `halopsa-cli recurring-invoice delete`  -  Delete specific InvoiceHeader. 				Requires authentication.
- `halopsa-cli recurring-invoice get`  -  Use this to return a single instance of InvoiceHeader. 				Requires authentication.
- `halopsa-cli recurring-invoice list`  -  Use this to return multiple InvoiceHeader. 				Requires authentication.

**recurring-item**  -  Manage recurring item

- `halopsa-cli recurring-item`  -  Use this to return multiple AreaItem. 				Requires authentication.

**release**  -  Manage release

- `halopsa-cli release create`  -  Create
- `halopsa-cli release delete`  -  Delete
- `halopsa-cli release get`  -  Use this to return a single instance of Release. 				Requires authentication.
- `halopsa-cli release list`  -  . 				Requires authentication.

**release-note-group**  -  Manage release note group

- `halopsa-cli release-note-group create`  -  Create
- `halopsa-cli release-note-group delete`  -  Delete
- `halopsa-cli release-note-group get`  -  Use this to return a single instance of ReleaseNoteGroup. 				Requires authentication.
- `halopsa-cli release-note-group list`  -  List

**release-pipeline**  -  Manage release pipeline

- `halopsa-cli release-pipeline create`  -  Create
- `halopsa-cli release-pipeline delete`  -  Delete
- `halopsa-cli release-pipeline get`  -  Get
- `halopsa-cli release-pipeline list`  -  List

**release-type**  -  Manage release type

- `halopsa-cli release-type create`  -  Create
- `halopsa-cli release-type delete`  -  Delete
- `halopsa-cli release-type get`  -  Use this to return a single instance of ReleaseType. 				Requires authentication.
- `halopsa-cli release-type list`  -  List

**remote-session**  -  Manage remote session

- `halopsa-cli remote-session create`  -  Create
- `halopsa-cli remote-session delete`  -  Delete
- `halopsa-cli remote-session get`  -  Use this to return a single instance of RemoteSessionData. 				Requires authentication.
- `halopsa-cli remote-session list`  -  Use this to return multiple RemoteSessionData. 				Requires authentication.

**remote-session-teams**  -  Manage remote session teams

- `halopsa-cli remote-session-teams`  -  Use this to return multiple RemoteSessionTeams. 				Requires authentication.

**report**  -  Manage report

- `halopsa-cli report create`  -  Create
- `halopsa-cli report create-bookmark`  -  Create bookmark
- `halopsa-cli report create-createpdf`  -  Create createpdf
- `halopsa-cli report create-print`  -  Create print
- `halopsa-cli report delete`  -  Delete
- `halopsa-cli report get`  -  Use this to return a single instance of AnalyzerProfile. 				Requires authentication.
- `halopsa-cli report list`  -  Use this to return multiple AnalyzerProfile. 				Requires authentication.

**report-data**  -  Manage report data

- `halopsa-cli report-data <publishedid>`  -  Get

**report-repository**  -  Manage report repository

- `halopsa-cli report-repository get`  -  Use this to return a single instance of AnalyzerProfile. 				Requires authentication.
- `halopsa-cli report-repository list`  -  Use this to return multiple AnalyzerProfile. 				Requires authentication.
- `halopsa-cli report-repository list-reportrepository`  -  Use this to return multiple Lookup. 				Requires authentication.

**resource-type**  -  Manage resource type

- `halopsa-cli resource-type get`  -  Get
- `halopsa-cli resource-type list`  -  List

**roadmap**  -  Manage roadmap

- `halopsa-cli roadmap`  -  . 				Requires authentication.

**roles**  -  Manage roles

- `halopsa-cli roles create`  -  Create
- `halopsa-cli roles delete`  -  Delete
- `halopsa-cli roles get`  -  Use this to return a single instance of NHD_Roles. 				Requires authentication.
- `halopsa-cli roles list`  -  Use this to return multiple NHD_Roles. 				Requires authentication.

**sage-business-cloud-details**  -  Manage sage business cloud details

- `halopsa-cli sage-business-cloud-details create`  -  Create
- `halopsa-cli sage-business-cloud-details delete`  -  Delete
- `halopsa-cli sage-business-cloud-details get`  -  Use this to return a single instance of SageBusinessCloudDetails. 				Requires authentication.
- `halopsa-cli sage-business-cloud-details list`  -  Use this to return multiple SageBusinessCloudDetails. 				Requires authentication.

**sail-point-details**  -  Manage sail point details

- `halopsa-cli sail-point-details create`  -  Create
- `halopsa-cli sail-point-details delete`  -  Delete
- `halopsa-cli sail-point-details get`  -  Get
- `halopsa-cli sail-point-details list`  -  List

**sail-point-role-mapping**  -  Manage sail point role mapping

- `halopsa-cli sail-point-role-mapping`  -  List

**sail-point-user-mapping**  -  Manage sail point user mapping

- `halopsa-cli sail-point-user-mapping`  -  List

**sales-mailbox**  -  Manage sales mailbox

- `halopsa-cli sales-mailbox create`  -  Create
- `halopsa-cli sales-mailbox delete`  -  Delete
- `halopsa-cli sales-mailbox get`  -  Use this to return a single instance of SalesMailbox. 				Requires authentication.
- `halopsa-cli sales-mailbox list`  -  List

**sales-mailbox-detail**  -  Manage sales mailbox detail

- `halopsa-cli sales-mailbox-detail create`  -  Create
- `halopsa-cli sales-mailbox-detail list`  -  List

**sales-order**  -  Manage sales order

- `halopsa-cli sales-order create`  -  Create
- `halopsa-cli sales-order create-salesorder`  -  Create salesorder
- `halopsa-cli sales-order delete`  -  Delete
- `halopsa-cli sales-order get`  -  Use this to return a single instance of OrderHead. 				Requires authentication.
- `halopsa-cli sales-order list`  -  Use this to return multiple OrderHead. 				Requires authentication.

**saved-forecast**  -  Manage saved forecast

- `halopsa-cli saved-forecast create`  -  Create
- `halopsa-cli saved-forecast delete`  -  Delete
- `halopsa-cli saved-forecast get`  -  Get
- `halopsa-cli saved-forecast list`  -  List

**schedule**  -  Manage schedule

- `halopsa-cli schedule create`  -  Create
- `halopsa-cli schedule get`  -  Use this to return a single instance of Schedule. 				Requires authentication.
- `halopsa-cli schedule list`  -  Use this to return multiple Schedule. 				Requires authentication.

**schedule-occurrence**  -  Manage schedule occurrence

- `halopsa-cli schedule-occurrence create`  -  Create
- `halopsa-cli schedule-occurrence get`  -  Get
- `halopsa-cli schedule-occurrence list`  -  List

**screen-layout**  -  Manage screen layout

- `halopsa-cli screen-layout create`  -  Create
- `halopsa-cli screen-layout delete`  -  Delete
- `halopsa-cli screen-layout get`  -  Use this to return a single instance of ScreenLayout. 				Requires authentication.
- `halopsa-cli screen-layout list`  -  Use this to return multiple ScreenLayout. 				Requires authentication.

**secure-secret-link**  -  Manage secure secret link

- `halopsa-cli secure-secret-link create`  -  Create
- `halopsa-cli secure-secret-link delete`  -  Delete
- `halopsa-cli secure-secret-link get`  -  Get
- `halopsa-cli secure-secret-link list`  -  List
- `halopsa-cli secure-secret-link list-securesecretlink`  -  List securesecretlink

**security-check**  -  Manage security check

- `halopsa-cli security-check list`  -  List
- `halopsa-cli security-check list-securitycheck`  -  List securitycheck

**security-question**  -  Manage security question

- `halopsa-cli security-question create`  -  Create
- `halopsa-cli security-question delete`  -  Delete
- `halopsa-cli security-question get`  -  Use this to return a single instance of SecurityQuestion. 				Requires authentication.
- `halopsa-cli security-question list`  -  List

**security-question-validate**  -  Manage security question validate

- `halopsa-cli security-question-validate create`  -  Create
- `halopsa-cli security-question-validate list`  -  List

**sentinel-one**  -  Manage sentinel one

- `halopsa-cli sentinel-one`  -  List

**sentinel-one-details**  -  Manage sentinel one details

- `halopsa-cli sentinel-one-details create`  -  Create
- `halopsa-cli sentinel-one-details delete`  -  Delete
- `halopsa-cli sentinel-one-details get`  -  Get
- `halopsa-cli sentinel-one-details list`  -  List

**service**  -  Manage service

- `halopsa-cli service create`  -  Create
- `halopsa-cli service create-unsubscribe`  -  Create unsubscribe
- `halopsa-cli service delete`  -  Delete
- `halopsa-cli service get`  -  Use this to return a single instance of ServSite. 				Requires authentication.
- `halopsa-cli service list`  -  Use this to return multiple ServSite. 				Requires authentication.

**service-availability**  -  Manage service availability

- `halopsa-cli service-availability create`  -  Create
- `halopsa-cli service-availability delete`  -  Delete
- `halopsa-cli service-availability get`  -  Get
- `halopsa-cli service-availability list`  -  List

**service-category**  -  Manage service category

- `halopsa-cli service-category create`  -  Create
- `halopsa-cli service-category delete`  -  Delete
- `halopsa-cli service-category get`  -  Use this to return a single instance of ServiceCategory. 				Requires authentication.
- `halopsa-cli service-category list`  -  Use this to return multiple ServiceCategory. 				Requires authentication.

**service-request-details**  -  Manage service request details

- `halopsa-cli service-request-details get`  -  Use this to return a single instance of ServiceRequestDetails. 				Requires authentication.
- `halopsa-cli service-request-details list`  -  Use this to return multiple ServiceRequestDetails. 				Requires authentication.

**service-restriction**  -  Manage service restriction

- `halopsa-cli service-restriction`  -  Use this to return multiple ServiceRestriction. 				Requires authentication.

**service-status**  -  Manage service status

- `halopsa-cli service-status create`  -  Create
- `halopsa-cli service-status create-servicestatus`  -  Create servicestatus
- `halopsa-cli service-status delete`  -  Delete
- `halopsa-cli service-status get`  -  Use this to return a single instance of ServStatus. 				Requires authentication.
- `halopsa-cli service-status get-servicestatus`  -  Get servicestatus
- `halopsa-cli service-status list`  -  Use this to return multiple ServStatus. 				Requires authentication.

**setup-tab**  -  Manage setup tab

- `halopsa-cli setup-tab create`  -  Create
- `halopsa-cli setup-tab get`  -  Use this to return a single instance of SetupTab. 				Requires authentication.
- `halopsa-cli setup-tab list`  -  List

**setup-tab-group**  -  Manage setup tab group

- `halopsa-cli setup-tab-group get`  -  Use this to return a single instance of SetupTabGroup. 				Requires authentication.
- `halopsa-cli setup-tab-group list`  -  List

**share-point**  -  Manage share point

- `halopsa-cli share-point`  -  List

**shopify-details**  -  Manage shopify details

- `halopsa-cli shopify-details create`  -  Create
- `halopsa-cli shopify-details delete`  -  Delete
- `halopsa-cli shopify-details get`  -  Get
- `halopsa-cli shopify-details list`  -  List

**single-sign-on-application**  -  Manage single sign on application

- `halopsa-cli single-sign-on-application create`  -  Create
- `halopsa-cli single-sign-on-application delete`  -  Delete
- `halopsa-cli single-sign-on-application get`  -  Get
- `halopsa-cli single-sign-on-application list`  -  List

**single-sign-on-attempt**  -  Manage single sign on attempt

- `halopsa-cli single-sign-on-attempt delete`  -  Delete
- `halopsa-cli single-sign-on-attempt get`  -  Get
- `halopsa-cli single-sign-on-attempt list`  -  List

**site**  -  Manage site

- `halopsa-cli site create`  -  Create
- `halopsa-cli site delete`  -  Delete
- `halopsa-cli site get`  -  Use this to return a single instance of Site. 				Requires authentication.
- `halopsa-cli site list`  -  Use this to return multiple Site. 				Requires authentication.
- `halopsa-cli site list-stockbins`  -  List stockbins

**sla**  -  Manage sla

- `halopsa-cli sla create`  -  Create
- `halopsa-cli sla delete`  -  Delete
- `halopsa-cli sla get`  -  Use this to return a single instance of SlaHead. 				Requires authentication.
- `halopsa-cli sla list`  -  Use this to return multiple SlaHead. 				Requires authentication.

**slack**  -  Manage slack

- `halopsa-cli slack create`  -  Create
- `halopsa-cli slack create-event`  -  Create event
- `halopsa-cli slack create-interactivity`  -  Create interactivity
- `halopsa-cli slack create-manifest`  -  Create manifest

**slack-chat-app**  -  Manage slack chat app

- `halopsa-cli slack-chat-app create`  -  Create
- `halopsa-cli slack-chat-app delete`  -  Delete
- `halopsa-cli slack-chat-app get`  -  Get
- `halopsa-cli slack-chat-app list`  -  List

**slack-details**  -  Manage slack details

- `halopsa-cli slack-details create`  -  Create
- `halopsa-cli slack-details create-slackdetails`  -  Create slackdetails
- `halopsa-cli slack-details delete`  -  Delete
- `halopsa-cli slack-details get`  -  Use this to return a single instance of SlackDetails. 				Requires authentication.
- `halopsa-cli slack-details list`  -  Use this to return multiple SlackDetails. 				Requires authentication.

**snipe-itdetails**  -  Manage snipe itdetails

- `halopsa-cli snipe-itdetails create`  -  Create
- `halopsa-cli snipe-itdetails delete`  -  Delete
- `halopsa-cli snipe-itdetails get`  -  Get
- `halopsa-cli snipe-itdetails list`  -  List

**snow-details**  -  Manage snow details

- `halopsa-cli snow-details create`  -  Create
- `halopsa-cli snow-details delete`  -  Delete
- `halopsa-cli snow-details get`  -  Use this to return a single instance of SnowDetails. 				Requires authentication.
- `halopsa-cli snow-details list`  -  Use this to return multiple SnowDetails. 				Requires authentication.

**software-licence**  -  Manage software licence

- `halopsa-cli software-licence create`  -  Create
- `halopsa-cli software-licence delete`  -  Delete
- `halopsa-cli software-licence get`  -  Use this to return a single instance of Licence. 				Requires authentication.
- `halopsa-cli software-licence list`  -  Use this to return multiple Licence. 				Requires authentication.

**software-licence-role**  -  Manage software licence role

- `halopsa-cli software-licence-role`  -  Use this to return multiple LicenceRole. 				Requires authentication.

**sophos**  -  Manage sophos

- `halopsa-cli sophos`  -  List

**sophos-details**  -  Manage sophos details

- `halopsa-cli sophos-details create`  -  Create
- `halopsa-cli sophos-details delete`  -  Delete
- `halopsa-cli sophos-details get`  -  Get
- `halopsa-cli sophos-details list`  -  List

**sqlimport**  -  Manage sqlimport

- `halopsa-cli sqlimport create`  -  Create
- `halopsa-cli sqlimport delete`  -  Delete
- `halopsa-cli sqlimport get`  -  Use this to return a single instance of SQLImport. 				Requires authentication.
- `halopsa-cli sqlimport list`  -  Use this to return multiple SQLImport. 				Requires authentication.

**status**  -  Manage status

- `halopsa-cli status create`  -  Create
- `halopsa-cli status delete`  -  Delete
- `halopsa-cli status get`  -  Use this to return a single instance of TStatus. 				Requires authentication.
- `halopsa-cli status list`  -  Use this to return multiple TStatus. 				Requires authentication.

**stock-bin**  -  Manage stock bin

- `halopsa-cli stock-bin create`  -  Create
- `halopsa-cli stock-bin delete`  -  Delete
- `halopsa-cli stock-bin get`  -  Get
- `halopsa-cli stock-bin list`  -  List

**stock-trace**  -  Manage stock trace

- `halopsa-cli stock-trace get`  -  Get
- `halopsa-cli stock-trace list`  -  List

**stream-one-ion-details**  -  Manage stream one ion details

- `halopsa-cli stream-one-ion-details create`  -  Create
- `halopsa-cli stream-one-ion-details delete`  -  Delete
- `halopsa-cli stream-one-ion-details get`  -  Get
- `halopsa-cli stream-one-ion-details list`  -  List

**style-profile**  -  Manage style profile

- `halopsa-cli style-profile create`  -  Create
- `halopsa-cli style-profile delete`  -  Delete
- `halopsa-cli style-profile get`  -  Get
- `halopsa-cli style-profile list`  -  List

**supplier**  -  Manage supplier

- `halopsa-cli supplier create`  -  Create
- `halopsa-cli supplier delete`  -  Delete
- `halopsa-cli supplier get`  -  Use this to return a single instance of Company. 				Requires authentication.
- `halopsa-cli supplier list`  -  Use this to return multiple Company. 				Requires authentication.

**supplier-contract**  -  Manage supplier contract

- `halopsa-cli supplier-contract create`  -  Create
- `halopsa-cli supplier-contract create-suppliercontract`  -  Create suppliercontract
- `halopsa-cli supplier-contract delete`  -  Delete
- `halopsa-cli supplier-contract get`  -  Use this to return a single instance of Contract. 				Requires authentication.
- `halopsa-cli supplier-contract list`  -  Use this to return multiple Contract. 				Requires authentication.

**synnex-details**  -  Manage synnex details

- `halopsa-cli synnex-details create`  -  Create
- `halopsa-cli synnex-details delete`  -  Delete
- `halopsa-cli synnex-details get`  -  Use this to return a single instance of IngramMicroDetails. 				Requires authentication.
- `halopsa-cli synnex-details list`  -  List

**tabs**  -  Manage tabs

- `halopsa-cli tabs create`  -  Create
- `halopsa-cli tabs delete`  -  Delete
- `halopsa-cli tabs get`  -  Use this to return a single instance of Tabname. 				Requires authentication.
- `halopsa-cli tabs list`  -  Use this to return multiple Tabname. 				Requires authentication.

**tags**  -  Manage tags

- `halopsa-cli tags create`  -  Create
- `halopsa-cli tags delete`  -  Delete
- `halopsa-cli tags get`  -  Use this to return a single instance of Tag. 				Requires authentication.
- `halopsa-cli tags list`  -  List

**take-control**  -  Manage take control

- `halopsa-cli take-control`  -  List

**tanium-details**  -  Manage tanium details

- `halopsa-cli tanium-details create`  -  Create
- `halopsa-cli tanium-details delete`  -  Delete
- `halopsa-cli tanium-details get`  -  Get
- `halopsa-cli tanium-details list`  -  List

**task-monitor-event**  -  Manage task monitor event

- `halopsa-cli task-monitor-event`  -  List

**task-schedule**  -  Manage task schedule

- `halopsa-cli task-schedule create`  -  Create
- `halopsa-cli task-schedule list`  -  List

**task-trace**  -  Manage task trace

- `halopsa-cli task-trace get`  -  Get
- `halopsa-cli task-trace list`  -  List

**tax**  -  Manage tax

- `halopsa-cli tax create`  -  Create
- `halopsa-cli tax delete`  -  Delete
- `halopsa-cli tax get`  -  Use this to return a single instance of Tax. 				Requires authentication.
- `halopsa-cli tax list`  -  Use this to return multiple Tax. 				Requires authentication.

**tax-rule**  -  Manage tax rule

- `halopsa-cli tax-rule create`  -  Create
- `halopsa-cli tax-rule delete`  -  Delete
- `halopsa-cli tax-rule get`  -  Get
- `halopsa-cli tax-rule list`  -  List

**team**  -  Manage team

- `halopsa-cli team create`  -  Create
- `halopsa-cli team delete`  -  Delete
- `halopsa-cli team get`  -  Use this to return a single instance of SectionDetail. 				Requires authentication.
- `halopsa-cli team list`  -  Use this to return multiple SectionDetail. 				Requires authentication.
- `halopsa-cli team list-tree`  -  List tree

**team-image**  -  Manage team image

- `halopsa-cli team-image <id>`  -  Get

**tech-data-reseller-details**  -  Manage tech data reseller details

- `halopsa-cli tech-data-reseller-details create`  -  Create
- `halopsa-cli tech-data-reseller-details delete`  -  Delete
- `halopsa-cli tech-data-reseller-details get`  -  Get
- `halopsa-cli tech-data-reseller-details list`  -  List

**template**  -  Manage template

- `halopsa-cli template create`  -  Create
- `halopsa-cli template delete`  -  Delete
- `halopsa-cli template get`  -  Use this to return a single instance of StdRequest. 				Requires authentication.
- `halopsa-cli template list`  -  Use this to return multiple StdRequest. 				Requires authentication.

**tenable**  -  Manage tenable

- `halopsa-cli tenable create`  -  Create
- `halopsa-cli tenable create-export`  -  Create export
- `halopsa-cli tenable list`  -  List
- `halopsa-cli tenable list-status`  -  List status

**tenable-details**  -  Manage tenable details

- `halopsa-cli tenable-details create`  -  Create
- `halopsa-cli tenable-details delete`  -  Delete
- `halopsa-cli tenable-details get`  -  Get
- `halopsa-cli tenable-details list`  -  List

**tenant**  -  Manage tenant

- `halopsa-cli tenant create`  -  Create
- `halopsa-cli tenant list`  -  List

**test-error**  -  Manage test error

- `halopsa-cli test-error`  -  List

**test1**  -  Manage test1

- `halopsa-cli test1`  -  List

**test3**  -  Manage test3

- `halopsa-cli test3`  -  List

**test4**  -  Manage test4

- `halopsa-cli test4`  -  List

**ticket-approval**  -  Manage ticket approval

- `halopsa-cli ticket-approval create`  -  Create
- `halopsa-cli ticket-approval delete`  -  Delete
- `halopsa-cli ticket-approval get`  -  Use this to return a single instance of FaultApproval. 				Requires authentication.
- `halopsa-cli ticket-approval list`  -  Use this to return multiple FaultApproval. 				Requires authentication.

**ticket-area**  -  Manage ticket area

- `halopsa-cli ticket-area create`  -  Create
- `halopsa-cli ticket-area delete`  -  Delete
- `halopsa-cli ticket-area get`  -  Use this to return a single instance of TicketArea. 				Requires authentication.
- `halopsa-cli ticket-area list`  -  List

**ticket-rules**  -  Manage ticket rules

- `halopsa-cli ticket-rules create`  -  Create
- `halopsa-cli ticket-rules delete`  -  Delete
- `halopsa-cli ticket-rules get`  -  Use this to return a single instance of Autoassign. 				Requires authentication.
- `halopsa-cli ticket-rules list`  -  Use this to return multiple Autoassign. 				Requires authentication.

**ticket-type**  -  Manage ticket type

- `halopsa-cli ticket-type create`  -  Create
- `halopsa-cli ticket-type delete`  -  Delete
- `halopsa-cli ticket-type get`  -  Use this to return a single instance of RequestType. 				Requires authentication.
- `halopsa-cli ticket-type list`  -  Use this to return multiple RequestType. 				Requires authentication.

**ticket-type-field**  -  Manage ticket type field

- `halopsa-cli ticket-type-field`  -  Use this to return multiple RequestTypeField. 				Requires authentication.

**ticket-type-group**  -  Manage ticket type group

- `halopsa-cli ticket-type-group create`  -  Create
- `halopsa-cli ticket-type-group delete`  -  Delete
- `halopsa-cli ticket-type-group get`  -  Use this to return a single instance of RequestTypeGroup. 				Requires authentication.
- `halopsa-cli ticket-type-group list`  -  List

**tickets**  -  Manage tickets

- `halopsa-cli tickets create`  -  Create
- `halopsa-cli tickets create-object`  -  Create object
- `halopsa-cli tickets create-processchildren`  -  Create processchildren
- `halopsa-cli tickets create-setbillableproject`  -  Create setbillableproject
- `halopsa-cli tickets create-view`  -  Create view
- `halopsa-cli tickets create-vote`  -  Create vote
- `halopsa-cli tickets delete`  -  Delete specific Faults. 				Requires authentication.
- `halopsa-cli tickets get`  -  Use this to return a single instance of Faults. 				Requires authentication.
- `halopsa-cli tickets list`  -  Use this to return multiple Faults. 				Requires authentication.
- `halopsa-cli tickets list-salesmailbox`  -  List salesmailbox
- `halopsa-cli tickets list-zapier`  -  List zapier

**timesheet**  -  Manage timesheet

- `halopsa-cli timesheet create`  -  Create
- `halopsa-cli timesheet get`  -  Use this to return a single instance of Timesheet. 				Requires authentication.
- `halopsa-cli timesheet list`  -  List
- `halopsa-cli timesheet list-forecasting`  -  List forecasting
- `halopsa-cli timesheet list-mine`  -  List mine

**timesheet-event**  -  Manage timesheet event

- `halopsa-cli timesheet-event create`  -  Create
- `halopsa-cli timesheet-event delete`  -  Delete
- `halopsa-cli timesheet-event get`  -  Use this to return a single instance of TimesheetEvent. 				Requires authentication.
- `halopsa-cli timesheet-event list`  -  Use this to return multiple TimesheetEvent. 				Requires authentication.
- `halopsa-cli timesheet-event list-timesheetevent`  -  List timesheetevent

**timeslot**  -  Manage timeslot

- `halopsa-cli timeslot`  -  Use this to return multiple Timeslot. 				Requires authentication.

**to-do**  -  Manage to do

- `halopsa-cli to-do create`  -  Create
- `halopsa-cli to-do list`  -  Use this to return multiple FaultToDo. 				Requires authentication.

**to-do-group**  -  Manage to do group

- `halopsa-cli to-do-group create`  -  Create
- `halopsa-cli to-do-group delete`  -  Delete
- `halopsa-cli to-do-group get`  -  Get
- `halopsa-cli to-do-group list`  -  List

**top-level**  -  Manage top level

- `halopsa-cli top-level create`  -  Create
- `halopsa-cli top-level delete`  -  Delete
- `halopsa-cli top-level get`  -  Use this to return a single instance of Tree. 				Requires authentication.
- `halopsa-cli top-level list`  -  Use this to return multiple Tree. 				Requires authentication.

**transcription-store**  -  Manage transcription store

- `halopsa-cli transcription-store create`  -  Create
- `halopsa-cli transcription-store delete`  -  Delete
- `halopsa-cli transcription-store get`  -  Get
- `halopsa-cli transcription-store list`  -  List

**translation**  -  Manage translation

- `halopsa-cli translation create`  -  Create
- `halopsa-cli translation list`  -  List

**twilio**  -  Manage twilio

- `halopsa-cli twilio create`  -  Create
- `halopsa-cli twilio create-twiml`  -  Create twiml

**twilio-details**  -  Manage twilio details

- `halopsa-cli twilio-details`  -  List

**twilio-whats-app-details**  -  Manage twilio whats app details

- `halopsa-cli twilio-whats-app-details create`  -  Create
- `halopsa-cli twilio-whats-app-details delete`  -  Delete
- `halopsa-cli twilio-whats-app-details get`  -  Get
- `halopsa-cli twilio-whats-app-details list`  -  List

**twitter-details**  -  Manage twitter details

- `halopsa-cli twitter-details create`  -  Create
- `halopsa-cli twitter-details delete`  -  Delete
- `halopsa-cli twitter-details get`  -  Use this to return a single instance of TwitterDetails. 				Requires authentication.
- `halopsa-cli twitter-details list`  -  Use this to return multiple TwitterDetails. 				Requires authentication.

**unsub-service-emails**  -  Manage unsub service emails

- `halopsa-cli unsub-service-emails create`  -  Create
- `halopsa-cli unsub-service-emails delete`  -  Delete
- `halopsa-cli unsub-service-emails get`  -  Use this to return a single instance of UnsubEmailServiceUsers. 				Requires authentication.
- `halopsa-cli unsub-service-emails list`  -  List

**user-change**  -  Manage user change

- `halopsa-cli user-change`  -  Use this to return multiple UserChange. 				Requires authentication.

**user-roles**  -  Manage user roles

- `halopsa-cli user-roles create`  -  Create
- `halopsa-cli user-roles delete`  -  Delete
- `halopsa-cli user-roles get`  -  Use this to return a single instance of UserRoles. 				Requires authentication.
- `halopsa-cli user-roles list`  -  List

**users**  -  Manage users

- `halopsa-cli users create`  -  Create
- `halopsa-cli users create-prefs`  -  Create prefs
- `halopsa-cli users delete`  -  Delete
- `halopsa-cli users get`  -  Use this to return a single instance of Users. 				Requires authentication.
- `halopsa-cli users list`  -  Use this to return multiple Users. 				Requires authentication.
- `halopsa-cli users list-me`  -  List me

**version-info**  -  Manage version info

- `halopsa-cli version-info get`  -  Use this to return a single instance of Release. 				Requires authentication.
- `halopsa-cli version-info get-versioninfo`  -  Get versioninfo
- `halopsa-cli version-info list`  -  . 				Requires authentication.
- `halopsa-cli version-info list-versioninfo`  -  List versioninfo
- `halopsa-cli version-info list-versioninfo-2`  -  . 				Requires authentication.
- `halopsa-cli version-info list-versioninfo-3`  -  . 				Requires authentication.

**view-columns**  -  Manage view columns

- `halopsa-cli view-columns create`  -  Create
- `halopsa-cli view-columns delete`  -  Delete
- `halopsa-cli view-columns get`  -  Use this to return a single instance of ViewColumns. 				Requires authentication.
- `halopsa-cli view-columns list`  -  Use this to return multiple ViewColumns. 				Requires authentication.

**view-filter**  -  Manage view filter

- `halopsa-cli view-filter create`  -  Create
- `halopsa-cli view-filter delete`  -  Delete
- `halopsa-cli view-filter get`  -  Use this to return a single instance of ViewFilter. 				Requires authentication.
- `halopsa-cli view-filter list`  -  Use this to return multiple ViewFilter. 				Requires authentication.

**view-list-group**  -  Manage view list group

- `halopsa-cli view-list-group create`  -  Create
- `halopsa-cli view-list-group delete`  -  Delete
- `halopsa-cli view-list-group get`  -  Use this to return a single instance of ViewListGroup. 				Requires authentication.
- `halopsa-cli view-list-group list`  -  Use this to return multiple ViewListGroup. 				Requires authentication.

**view-lists**  -  Manage view lists

- `halopsa-cli view-lists create`  -  Create
- `halopsa-cli view-lists delete`  -  Delete
- `halopsa-cli view-lists get`  -  Use this to return a single instance of ViewLists. 				Requires authentication.
- `halopsa-cli view-lists list`  -  Use this to return multiple ViewLists. 				Requires authentication.

**virima**  -  Manage virima

- `halopsa-cli virima`  -  List

**virima-details**  -  Manage virima details

- `halopsa-cli virima-details create`  -  Create
- `halopsa-cli virima-details delete`  -  Delete
- `halopsa-cli virima-details get`  -  Get
- `halopsa-cli virima-details list`  -  List

**virtual-agent**  -  Manage virtual agent

- `halopsa-cli virtual-agent create`  -  Create
- `halopsa-cli virtual-agent delete`  -  Delete
- `halopsa-cli virtual-agent get`  -  Get
- `halopsa-cli virtual-agent list`  -  List

**vmworkspace-details**  -  Manage vmworkspace details

- `halopsa-cli vmworkspace-details create`  -  Create
- `halopsa-cli vmworkspace-details delete`  -  Delete
- `halopsa-cli vmworkspace-details get`  -  Get
- `halopsa-cli vmworkspace-details list`  -  List

**vorboss**  -  Manage vorboss

- `halopsa-cli vorboss`  -  List

**webhook**  -  Manage webhook

- `halopsa-cli webhook create`  -  Create
- `halopsa-cli webhook delete`  -  Delete
- `halopsa-cli webhook get`  -  Use this to return a single instance of Webhook. 				Requires authentication.
- `halopsa-cli webhook list`  -  Use this to return multiple Webhook. 				Requires authentication.

**webhook-event**  -  Manage webhook event

- `halopsa-cli webhook-event create`  -  Create
- `halopsa-cli webhook-event get`  -  Use this to return a single instance of WebhookEvent. 				Requires authentication.
- `halopsa-cli webhook-event list`  -  Use this to return multiple WebhookEvent. 				Requires authentication.

**webhook-repository**  -  Manage webhook repository

- `halopsa-cli webhook-repository get`  -  Use this to return a single instance of Webhook. 				Requires authentication.
- `halopsa-cli webhook-repository list`  -  Use this to return multiple Webhook. 				Requires authentication.

**whats-app**  -  Manage whats app

- `halopsa-cli whats-app list`  -  List
- `halopsa-cli whats-app list-whatsapp`  -  List whatsapp

**wordpress-details**  -  Manage wordpress details

- `halopsa-cli wordpress-details create`  -  Create
- `halopsa-cli wordpress-details delete`  -  Delete
- `halopsa-cli wordpress-details get`  -  Get
- `halopsa-cli wordpress-details list`  -  List

**wordpress-org-details**  -  Manage wordpress org details

- `halopsa-cli wordpress-org-details create`  -  Create
- `halopsa-cli wordpress-org-details delete`  -  Delete
- `halopsa-cli wordpress-org-details get`  -  Get
- `halopsa-cli wordpress-org-details list`  -  List

**workday**  -  Manage workday

- `halopsa-cli workday create`  -  Create
- `halopsa-cli workday delete`  -  Delete
- `halopsa-cli workday get`  -  Use this to return a single instance of Workdays. 				Requires authentication.
- `halopsa-cli workday list`  -  Use this to return multiple Workdays. 				Requires authentication.

**workflow-target**  -  Manage workflow target

- `halopsa-cli workflow-target create`  -  Create
- `halopsa-cli workflow-target delete`  -  Delete
- `halopsa-cli workflow-target get`  -  Get
- `halopsa-cli workflow-target list`  -  List

**workflows**  -  Manage workflows

- `halopsa-cli workflows create`  -  Create
- `halopsa-cli workflows delete`  -  Delete
- `halopsa-cli workflows get`  -  Use this to return a single instance of FlowHeader. 				Requires authentication.
- `halopsa-cli workflows list`  -  Use this to return multiple FlowHeader. 				Requires authentication.

**workflowstep**  -  Manage workflowstep

- `halopsa-cli workflowstep`  -  Use this to return multiple FlowDetail. 				Requires authentication.

**xero-details**  -  Manage xero details

- `halopsa-cli xero-details create`  -  Create
- `halopsa-cli xero-details delete`  -  Delete
- `halopsa-cli xero-details get`  -  Use this to return a single instance of XeroDetails. 				Requires authentication.
- `halopsa-cli xero-details list`  -  Use this to return multiple XeroDetails. 				Requires authentication.

**xtype-role**  -  Manage xtype role

- `halopsa-cli xtype-role`  -  Use this to return multiple XTypeRole. 				Requires authentication.

**zendesk**  -  Manage zendesk

- `halopsa-cli zendesk`  -  List

**zoom**  -  Manage zoom

- `halopsa-cli zoom`  -  Create


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
halopsa-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Monday queue cleanse

```bash
halopsa-cli tickets age-out --status "Awaiting Customer Reply" --stale-days 14 --action-note "Auto-closing per policy"
```

Preview every stale customer-waiting ticket. Add --apply when you're ready to close them.

### Friday SLA radar before hand-off

```bash
halopsa-cli sla breaching --within 24h --team Support --agent --select id,summary,client_name,agent_name,minutes_to_breach
```

List every ticket the on-call shift inherits that's at risk of breaching SLA in their first 24h. Pipe to anything.

### Pre-call client briefing

```bash
halopsa-cli client card "Acme Corp" --agent --select active_tickets,contract_hours_remaining,assets,recent_kb_links
```

Get the client's complete situation in one query before answering the phone. Use --select to narrow what an agent sees.

### Friday timesheet reconcile

```bash
halopsa-cli time gaps --agent=me --week current --json
```

Find every ticket you touched this week with zero time logged so the gap doesn't ship with your timesheet.

### Contract overage check before client meeting

```bash
halopsa-cli contracts burn --client "Acme Corp" --month current --json
```

See current hours consumed vs. bank with projected overage so the contract conversation isn't a surprise.

## Auth Setup

Halo uses OAuth2 client_credentials. Create an API application in your tenant under Configuration > Integrations > Halo PSA API (Authentication Method: Client ID and Secret  -  Services), set `HALOPSA_TENANT=<yoursub>` in your env, then run `halopsa-cli auth login --client-id <id> --client-secret <secret>`. The CLI exchanges the credentials at https://<tenant>.halopsa.com/auth/token and caches the access token (auto-refreshed before expiry).

Run `halopsa-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  halopsa-cli actions list --agent --select id,name,status
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
halopsa-cli feedback "the --since flag is inclusive but docs say exclusive"
halopsa-cli feedback --stdin < notes.txt
halopsa-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/halopsa-cli/feedback.jsonl`. They are never POSTed unless `HALOPSA_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `HALOPSA_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
halopsa-cli profile save briefing --json
halopsa-cli --profile briefing actions list
halopsa-cli profile list --json
halopsa-cli profile show briefing
halopsa-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `halopsa-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/project-management/halopsa/cmd/halopsa-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add halopsa-mcp -- halopsa-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which halopsa-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   halopsa-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `halopsa-cli <command> --help`.
