---
name: hubspot
description: "Every Sales Hub feature, plus offline cross-object queries and retained property-change history. Trigger phrases: `find meetings ever scheduled`, `monthly meeting outcome report`, `hubspot stale leads`, `who do I call today hubspot`, `engagements timeline for this contact`, `use hubspot-cli`, `run hubspot`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "HubSpot"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - hubspot-cli
    install:
      - kind: script
        bins: [hubspot-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hubspot/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hubspot/install.ps1
---

# HubSpot - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `hubspot-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hubspot/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hubspot/install.ps1 | iex
   ```
3. Verify: `hubspot-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `hubspot-cli` and `hubspot-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

A local SQLite data layer no other HubSpot tool has: HubSpot's own CLI (`hs`) only covers CMS  -  there has never been a sales/CRM CLI from HubSpot itself. This one mirrors your CRM into local SQLite so commands like `nurture-mine`, `stale deals`, `owner-load`, and `pipeline-health` answer cross-table questions instantly and offline. New in this reprint: `sync --with-history` persists per-property snapshots into a shared property-history table, and `meetings ever-had` / `meetings status-report` answer questions HubSpot's standard search API physically cannot  -  'every meeting that was EVER status X in month Y, even after it flipped.'

## When to Use This CLI

Use hubspot-cli whenever an agent or human needs Sales Hub data without paying for a live API round trip per query. It is the right pick for nurture loops, pipeline-health dashboards, stale-prospect detection, bulk property updates from CSV, monthly property-history reports (`meetings status-report`, `meetings ever-had`), and any cross-object join (engagements × deals × owners) that the HubSpot web UI cannot answer in one screen. ANTI-TRIGGERS  -  do NOT use this CLI for: HubSpot CMS / themes / serverless functions (use the official `hs` CLI), Marketing emails / campaigns / forms / ads, Conversations / Inbox / Support workflows, Commerce Hub (orders / invoices / subscriptions / payments / carts / contracts).

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Local state that compounds

- **`stale`**  -  Find contacts or deals with no engagement in N days, scoped by owner or pipeline stage  -  instantly, offline.

  _Use this when the user asks 'what's gone cold'  -  works after one sync, no API quota burn._

  ```bash
  hubspot-cli stale deals --days 21 --owner me --json
  ```
- **`owner-load`**  -  Open deals per rep per pipeline stage with $ totals, count, and oldest-deal age  -  the Monday-morning sales-lead report.

  _Pick this over per-rep API calls when surfacing 'who has too many in-flight deals' or 'where is rep capacity hot'._

  ```bash
  hubspot-cli owner-load --pipeline default --json
  ```
- **`pipeline-health`**  -  Per-stage rollup of count, $ total, $ at risk (idle deals near their close date), and the oldest-stuck deal  -  one query for a sales-ops dashboard. Closed Lost weighted at probability 0, Closed Won at 1.0.

  _Use this for forecast-vs-reality checks before a pipeline review._

  ```bash
  hubspot-cli pipeline-health default --idle-days 14 --json
  ```
- **`nurture queue`**  -  Ranked 'who to contact today' list scored by stale-days × deal amount × stage probability, with the rationale exposed as columns.

  _Reach for this in the nurture skill or any daily-touch-list loop where an agent needs a priority order with reasons attached._

  ```bash
  hubspot-cli nurture queue --owner me --top 20 --agent
  ```
- **`deals top`**  -  Composite-ranked top-N deals by (signal × amount × stage-probability × inverse-days-since-contact) with the score breakdown exposed as columns.

  _Pick this for sales-lead Monday reviews or when an agent needs the top opportunities ranked by signal-weighted score, not raw amount._

  ```bash
  hubspot-cli deals top --top 5 --pipeline default --json
  ```

### Property history & audit

- **`meetings history`**  -  Show the full timeline of property changes for a single meeting (outcome, title, owner, custom fields)  -  when each value was set, by whom, and from what source.

  _Reach for this when investigating 'when did this meeting flip from Scheduled to No Show, and who changed it'. Requires a prior 'sync --resources hubspot-meetings-crm --with-history <props>' to populate the history table._

  ```bash
  hubspot-cli meetings history 53612340987612345 --json
  ```
- **`meetings ever-had`**  -  Find every meeting whose given property was EVER set to a given value within a date range  -  even if it has since changed.

  _Pick this when a customer or audit asks 'every meeting that was at some point in status X during month Y'. Requires a prior 'sync --with-history' capture; the standard /search API cannot answer it._

  ```bash
  hubspot-cli meetings ever-had --property hs_meeting_outcome --value Scheduled --from 2026-04-01 --to 2026-04-30 --json
  ```
- **`meetings status-report`**  -  Composes the meetings ever-had query into the canonical monthly-report shape: every meeting that touched the given status in the given month, with owner, title, current status, and the timestamp of the original status set.

  _Use this once per month per customer report  -  one command replaces a Python pull + a HubSpot export + a manual cross-reference. Requires a prior 'sync --resources hubspot-meetings-crm --with-history hs_meeting_outcome'._

  ```bash
  hubspot-cli meetings status-report --status scheduled --month 2026-04 --csv
  ```
- **`sync --with-history`**  -  Opt-in sync flag that requests propertiesWithHistory for the named properties on meetings (and on deals, contacts, companies when scoped to those). Persists per-property snapshots into the shared hubspot_property_history table.

  _Reach for this when you need property change history retained. Without --with-history sync only captures current values; with it, every read after that point can answer 'was this property ever value X' questions._

  ```bash
  hubspot-cli sync --resources hubspot-meetings-crm --with-history hs_meeting_outcome,hs_meeting_title,hubspot_owner_id
  ```
- **`deals velocity`**  -  Per-deal days-in-current-stage and per-stage median/p90 dwell time, computed from dealstage change history  -  find where deals rot.

  _Use this when the user asks where deals stall or how long deals sit per stage  -  requires a prior sync deals --with-history dealstage._

  ```bash
  hubspot-cli deals velocity --pipeline default --json
  ```

### Cross-object intelligence

- **`engagements of`**  -  Unified chronological timeline of every call, email, meeting, note, and task touching a contact, deal, or company.

  _Use this when an agent or human asks 'what touched this prospect/deal'  -  one command instead of five paginated API calls._

  ```bash
  hubspot-cli engagements of contact:12345 --since 30d --json
  ```
- **`notes signals`**  -  Scan note bodies for buying / lost signals (meeting scheduled, budget approved, no response, competitor chosen) and emit per-deal signal counts with the source note id.

  _Use this when summarizing what's heating up or cooling off in the pipeline without re-reading every note._

  ```bash
  hubspot-cli notes signals --pipeline default --since 30d --json
  ```
- **`since`**  -  What changed across contacts, deals, and engagements since a given timestamp  -  agent-friendly cross-object delta.

  _Reach for this in agent loops that need to react to CRM activity since the last run without paginating every collection._

  ```bash
  hubspot-cli since 24h --types deals,engagements --owner me --json
  ```
- **`contacts funnel`**  -  One-shot funnel table of contacts per lifecycle stage (subscriber → lead → MQL → SQL → opportunity → customer) with stage-to-stage conversion ratios.

  _Use this for 'where is the top of funnel leaking' questions  -  instant offline funnel snapshot after one sync._

  ```bash
  hubspot-cli contacts funnel --json
  ```
- **`deals unowned`**  -  Open deals with no owner or owned by a deactivated rep, with per-stage dollar exposure  -  the hygiene gap owner-load can't see.

  _Use this in the weekly hygiene pass to find orphaned deals before they rot  -  distinct from owner-load, which only aggregates owned deals._

  ```bash
  hubspot-cli deals unowned --pipeline default --json
  ```
- **`contacts win-back`**  -  Contacts attached to a Closed Won deal but with no engagement in N days  -  the customer-expansion and re-engage list.

  _Use this for post-win expansion outreach  -  distinct from nurture-mine (open-deal cold) and stale (any cold)._

  ```bash
  hubspot-cli contacts win-back --cold-days 90 --json
  ```
- **`deals forecast`**  -  Probability-weighted pipeline forecast bucketed by close-date month  -  the canonical GM revenue question, answered offline.

  _Use this for close-month weighted totals for revenue reviews; pipeline-health answers per-stage risk, forecast answers per-month expectation._

  ```bash
  hubspot-cli deals forecast --pipeline default --json
  ```

### Bulk operations

- **`nurture-mine`**  -  Surface the contacts assigned to you that have gone cold but still have open deals  -  the daily 'who do I call' list, computed across local SQLite.

  _Reach for this when an agent or human needs the daily Titans-style nurture queue without round-tripping HubSpot for every prospect._

  ```bash
  hubspot-cli nurture-mine --owner me --stale-days 14 --agent
  ```
- **`contacts bulk-update`**  -  Apply a CSV of property changes to many contacts at once, pre-validating each row against HubSpot's property schema (types, picklists) before any mutation.

  _Pick this over the HubSpot UI when sales-ops needs to update hundreds of contacts and demands a pre-flight error report rather than silent failures._

  ```bash
  hubspot-cli contacts bulk-update --from-csv people.csv --map email=Email,lifecyclestage=Stage --dry-run
  ```

## Command Reference

**batch**  -  Manage batch

- `hubspot-cli batch post-crm-v3-objects-object-type-archive-archive`  -  Archive a batch of objects by ID
- `hubspot-cli batch post-crm-v3-objects-object-type-create-create`  -  Create a batch of objects
- `hubspot-cli batch post-crm-v3-objects-object-type-read-read`  -  Retrieve records by record ID or include the `idProperty` parameter to retrieve records by a custom unique value
- `hubspot-cli batch post-crm-v3-objects-object-type-update-update`  -  Update a batch of objects by internal ID, or unique property values
- `hubspot-cli batch post-crm-v3-objects-object-type-upsert-upsert`  -  Create or update records identified by a unique property value as specified by the `idProperty` query param.

**crm**  -  Manage crm

- `hubspot-cli crm delete-v4-objects-object-type-object-id-associations-to-object-type-to-object-id-archive`  -  deletes all associations between two records.
- `hubspot-cli crm get-v4-objects-object-type-object-id-associations-to-object-type-get-page`  -  Retrieve all associations between a specific record and an object type. Limit 500 per call.
- `hubspot-cli crm post-v4-associations-from-object-type-to-object-type-batch-archive-archive`  -  Batch delete associations for objects
- `hubspot-cli crm post-v4-associations-from-object-type-to-object-type-batch-associate-default-create-default`  -  Create the default (most generic) association type between two object types
- `hubspot-cli crm post-v4-associations-from-object-type-to-object-type-batch-create-create`  -  Batch create associations for objects
- `hubspot-cli crm post-v4-associations-from-object-type-to-object-type-batch-labels-archive-archive-labels`  -  Batch delete specific association labels for objects.
- `hubspot-cli crm post-v4-associations-from-object-type-to-object-type-batch-read-get-page`  -  Batch read associations for objects to specific object type.
- `hubspot-cli crm post-v4-associations-usage-high-usage-report-user-id-request`  -  Requests a report of all objects in the portal which have a high usage of associations
- `hubspot-cli crm put-v4-objects-from-object-type-from-object-id-associations-default-to-object-type-to-object-id-create-default`  -  Create the default (most generic) association type between two object types
- `hubspot-cli crm put-v4-objects-object-type-object-id-associations-to-object-type-to-object-id-create`  -  Set association labels between two records.

**groups**  -  Manage groups

- `hubspot-cli groups delete-crm-v3-properties-object-type-name-archive`  -  Move a property group identified by {groupName} to the recycling bin.
- `hubspot-cli groups get-crm-v3-properties-object-type-get-all`  -  Read all existing property groups for the specified object type and HubSpot account.
- `hubspot-cli groups get-crm-v3-properties-object-type-name-get-by-name`  -  Read a property group identified by {groupName}.
- `hubspot-cli groups patch-crm-v3-properties-object-type-name-update`  -  Perform a partial update of a property group identified by {groupName}. Provided fields will be overwritten.
- `hubspot-cli groups post-crm-v3-properties-object-type-create`  -  Create and return a copy of a new property group.

**hubspot-calls-crm**  -  Manage hubspot calls crm

- `hubspot-cli hubspot-calls-crm delete-v3-objects-calls-call-id-archive`  -  Move an Object identified by `{callId}` to the recycling bin.
- `hubspot-cli hubspot-calls-crm get-v3-objects-calls-call-id-get-by-id`  -  Read an Object identified by `{callId}`.
- `hubspot-cli hubspot-calls-crm get-v3-objects-calls-get-page`  -  Read a page of calls. Control what is returned via the `properties` query param.
- `hubspot-cli hubspot-calls-crm patch-v3-objects-calls-call-id-update`  -  Perform a partial update of an Object identified by `{callId}`or optionally a unique property value as specified by the
- `hubspot-cli hubspot-calls-crm post-v3-objects-calls-batch-archive-archive`  -  Archive a batch of calls by ID.
- `hubspot-cli hubspot-calls-crm post-v3-objects-calls-batch-create-create`  -  Create a batch of calls.
- `hubspot-cli hubspot-calls-crm post-v3-objects-calls-batch-read-read`  -  Read a batch of calls by internal ID, or unique property values
- `hubspot-cli hubspot-calls-crm post-v3-objects-calls-batch-update-update`  -  Update a batch of calls by internal ID, or unique property values
- `hubspot-cli hubspot-calls-crm post-v3-objects-calls-batch-upsert-upsert`  -  Create or update records identified by a unique property value as specified by the `idProperty` query param.
- `hubspot-cli hubspot-calls-crm post-v3-objects-calls-create`  -  Create a call with the given properties and return a copy of the object, including the ID.
- `hubspot-cli hubspot-calls-crm post-v3-objects-calls-search-do-search`  -  Search for calls by filtering on properties, searching through associations, and sorting results.

**hubspot-companies-crm**  -  Manage hubspot companies crm

- `hubspot-cli hubspot-companies-crm delete-v3-objects-companies-company-id-archive`  -  Delete a company by ID. Deleted companies can be restored within 90 days of deletion.
- `hubspot-cli hubspot-companies-crm get-v3-objects-companies-company-id-get-by-id`  -  Retrieve a company by its ID (`companyId`) or by a unique property (`idProperty`).
- `hubspot-cli hubspot-companies-crm get-v3-objects-companies-get-page`  -  Retrieve all companies, using query parameters to control the information that gets returned.
- `hubspot-cli hubspot-companies-crm patch-v3-objects-companies-company-id-update`  -  Update a company by ID (`companyId`) or unique property value (`idProperty`).
- `hubspot-cli hubspot-companies-crm post-v3-objects-companies-batch-archive-archive`  -  Delete a batch of companies by ID. Deleted companies can be restored within 90 days of deletion.
- `hubspot-cli hubspot-companies-crm post-v3-objects-companies-batch-create-create`  -  Create a batch of companies.
- `hubspot-cli hubspot-companies-crm post-v3-objects-companies-batch-read-read`  -  Retrieve a batch of companies by ID (`companyId`) or by a unique property (`idProperty`).
- `hubspot-cli hubspot-companies-crm post-v3-objects-companies-batch-update-update`  -  Update a batch of companies by ID.
- `hubspot-cli hubspot-companies-crm post-v3-objects-companies-batch-upsert-upsert`  -  Create or update companies identified by a unique property value as specified by the `idProperty` query parameter.
- `hubspot-cli hubspot-companies-crm post-v3-objects-companies-create`  -  Create a single company. Include a `properties` object to define [property values](https://developers.hubspot.
- `hubspot-cli hubspot-companies-crm post-v3-objects-companies-merge-merge`  -  Merge two company records. Learn more about [merging records](https://knowledge.hubspot.com/records/merge-records).
- `hubspot-cli hubspot-companies-crm post-v3-objects-companies-search-do-search`  -  Search for companies by filtering on properties, searching through associations, and sorting results.

**hubspot-contacts-crm**  -  Manage hubspot contacts crm

- `hubspot-cli hubspot-contacts-crm delete-v3-objects-contacts-contact-id`  -  Move an Object identified by `{contactId}` to the recycling bin.
- `hubspot-cli hubspot-contacts-crm get-v3-objects-contacts`  -  Read a page of contacts. Control what is returned via the `properties` query param.
- `hubspot-cli hubspot-contacts-crm get-v3-objects-contacts-contact-id`  -  Read an Object identified by `{contactId}`.
- `hubspot-cli hubspot-contacts-crm patch-v3-objects-contacts-contact-id`  -  Perform a partial update of an Object identified by `{contactId}`.
- `hubspot-cli hubspot-contacts-crm post-v3-objects-contacts`  -  Create a contact with the given properties and return a copy of the object, including the ID.
- `hubspot-cli hubspot-contacts-crm post-v3-objects-contacts-batch-archive`  -  Archive a batch of contacts by ID
- `hubspot-cli hubspot-contacts-crm post-v3-objects-contacts-batch-create`  -  Create a batch of contacts
- `hubspot-cli hubspot-contacts-crm post-v3-objects-contacts-batch-read`  -  Read a batch of contacts by internal ID, or unique property values
- `hubspot-cli hubspot-contacts-crm post-v3-objects-contacts-batch-update`  -  Update a batch of contacts
- `hubspot-cli hubspot-contacts-crm post-v3-objects-contacts-gdpr-delete`  -  Permanently delete a contact and all associated content to follow GDPR.
- `hubspot-cli hubspot-contacts-crm post-v3-objects-contacts-merge`  -  Merge two contacts with same type
- `hubspot-cli hubspot-contacts-crm post-v3-objects-contacts-search`  -  Post v3 objects contacts search

**hubspot-deals-crm**  -  Manage hubspot deals crm

- `hubspot-cli hubspot-deals-crm delete-v3-objects-0-3-deal-id-archive`  -  Move an Object identified by `{dealId}` to the recycling bin.
- `hubspot-cli hubspot-deals-crm get-v3-objects-0-3-deal-id-get-by-id`  -  Read an Object identified by `{dealId}`.
- `hubspot-cli hubspot-deals-crm get-v3-objects-0-3-get-page`  -  Read a page of deals. Control what is returned via the `properties` query param.
- `hubspot-cli hubspot-deals-crm patch-v3-objects-0-3-deal-id-update`  -  Perform a partial update of an Object identified by `{dealId}`or optionally a unique property value as specified by the
- `hubspot-cli hubspot-deals-crm post-v3-objects-0-3-batch-archive-archive`  -  Archive multiple deals using their IDs.
- `hubspot-cli hubspot-deals-crm post-v3-objects-0-3-batch-create-create`  -  Create multiple deals in a single request.
- `hubspot-cli hubspot-deals-crm post-v3-objects-0-3-batch-read-read`  -  Retrieve records by record ID or include the `idProperty` parameter to retrieve records by a custom unique value
- `hubspot-cli hubspot-deals-crm post-v3-objects-0-3-batch-update-update`  -  Update multiple deals using their internal IDs or unique property values.
- `hubspot-cli hubspot-deals-crm post-v3-objects-0-3-batch-upsert-upsert`  -  Create or update records identified by a unique property value as specified by the `idProperty` query param.
- `hubspot-cli hubspot-deals-crm post-v3-objects-0-3-create`  -  Create a deal with the given properties and return a copy of the object, including the ID.
- `hubspot-cli hubspot-deals-crm post-v3-objects-0-3-merge-merge`  -  Combine two deals of the same type into a single deal.
- `hubspot-cli hubspot-deals-crm post-v3-objects-0-3-search-do-search`  -  Search for deals using various filters and criteria to retrieve specific records.

**hubspot-emails-crm**  -  Manage hubspot emails crm

- `hubspot-cli hubspot-emails-crm delete-v3-objects-emails-email-id-archive`  -  Move an Object identified by `{emailId}` to the recycling bin.
- `hubspot-cli hubspot-emails-crm get-v3-objects-emails-email-id-get-by-id`  -  Read an Object identified by `{emailId}`.
- `hubspot-cli hubspot-emails-crm get-v3-objects-emails-get-page`  -  Read a page of emails. Control what is returned via the `properties` query param.
- `hubspot-cli hubspot-emails-crm patch-v3-objects-emails-email-id-update`  -  Perform a partial update of an Object identified by `{emailId}`or optionally a unique property value as specified by
- `hubspot-cli hubspot-emails-crm post-v3-objects-emails-batch-archive-archive`  -  Archive a batch of emails identified by their IDs.
- `hubspot-cli hubspot-emails-crm post-v3-objects-emails-batch-create-create`  -  Create a batch of emails with specified properties and return the created objects.
- `hubspot-cli hubspot-emails-crm post-v3-objects-emails-batch-read-read`  -  Retrieve records by record ID or include the `idProperty` parameter to retrieve records by a custom unique value
- `hubspot-cli hubspot-emails-crm post-v3-objects-emails-batch-update-update`  -  Update a batch of emails using their internal IDs or unique property values.
- `hubspot-cli hubspot-emails-crm post-v3-objects-emails-batch-upsert-upsert`  -  Create or update records identified by a unique property value as specified by the `idProperty` query param.
- `hubspot-cli hubspot-emails-crm post-v3-objects-emails-create`  -  Create a email with the given properties and return a copy of the object, including the ID.
- `hubspot-cli hubspot-emails-crm post-v3-objects-emails-search-do-search`  -  Perform a search for emails based on the provided query parameters and return matching results.

**hubspot-imports-crm**  -  Manage hubspot imports crm

- `hubspot-cli hubspot-imports-crm get-v3-imports-import-id-errors-v3-imports-import-id-errors`  -  Get v3 imports import id errors v3 imports import id errors
- `hubspot-cli hubspot-imports-crm get-v3-imports-import-id-v3-imports-import-id`  -  Get v3 imports import id v3 imports import id
- `hubspot-cli hubspot-imports-crm get-v3-imports-v3-imports`  -  Get v3 imports v3 imports
- `hubspot-cli hubspot-imports-crm post-v3-imports-import-id-cancel-v3-imports-import-id-cancel`  -  Post v3 imports import id cancel v3 imports import id cancel
- `hubspot-cli hubspot-imports-crm post-v3-imports-v3-imports`  -  Post v3 imports v3 imports

**hubspot-leads-crm**  -  Manage hubspot leads crm

- `hubspot-cli hubspot-leads-crm delete-v3-objects-leads-leads-id-archive`  -  Move an Object identified by `{leadsId}` to the recycling bin.
- `hubspot-cli hubspot-leads-crm get-v3-objects-leads-get-page`  -  Read a page of leads. Control what is returned via the `properties` query param.
- `hubspot-cli hubspot-leads-crm get-v3-objects-leads-leads-id-get-by-id`  -  Read an Object identified by `{leadsId}`.
- `hubspot-cli hubspot-leads-crm patch-v3-objects-leads-leads-id-update`  -  Perform a partial update of an Object identified by `{leadsId}`or optionally a unique property value as specified by
- `hubspot-cli hubspot-leads-crm post-v3-objects-leads-batch-archive-archive`  -  Archive multiple leads by their IDs in a single request, moving them to the recycling bin.
- `hubspot-cli hubspot-leads-crm post-v3-objects-leads-batch-create-create`  -  Create multiple lead records in a single request by providing a batch of lead data.
- `hubspot-cli hubspot-leads-crm post-v3-objects-leads-batch-read-read`  -  Retrieve records by record ID or include the `idProperty` parameter to retrieve records by a custom unique value
- `hubspot-cli hubspot-leads-crm post-v3-objects-leads-batch-update-update`  -  Update multiple lead records using their internal IDs or unique property values.
- `hubspot-cli hubspot-leads-crm post-v3-objects-leads-batch-upsert-upsert`  -  Create or update records identified by a unique property value as specified by the `idProperty` query param.
- `hubspot-cli hubspot-leads-crm post-v3-objects-leads-create`  -  Create a lead with the given properties and return a copy of the object, including the ID.
- `hubspot-cli hubspot-leads-crm post-v3-objects-leads-search-do-search`  -  Perform a search for leads based on the provided filter groups, properties, and sorting options.

**hubspot-line-items-crm**  -  Manage hubspot line items crm

- `hubspot-cli hubspot-line-items-crm delete-v3-objects-line-items-line-item-id-archive`  -  Move an Object identified by `{lineItemId}` to the recycling bin.
- `hubspot-cli hubspot-line-items-crm get-v3-objects-line-items-get-page`  -  Read a page of line items. Control what is returned via the `properties` query param.
- `hubspot-cli hubspot-line-items-crm get-v3-objects-line-items-line-item-id-get-by-id`  -  Read an Object identified by `{lineItemId}`.
- `hubspot-cli hubspot-line-items-crm patch-v3-objects-line-items-line-item-id-update`  -  Perform a partial update of an Object identified by `{lineItemId}`or optionally a unique property value as specified by
- `hubspot-cli hubspot-line-items-crm post-v3-objects-line-items-batch-archive-archive`  -  Archive multiple line items simultaneously by specifying their IDs in the request body.
- `hubspot-cli hubspot-line-items-crm post-v3-objects-line-items-batch-create-create`  -  Create multiple line items in a single request by providing the necessary properties and associations for each item.
- `hubspot-cli hubspot-line-items-crm post-v3-objects-line-items-batch-read-read`  -  Retrieve records by record ID or include the `idProperty` parameter to retrieve records by a custom unique value
- `hubspot-cli hubspot-line-items-crm post-v3-objects-line-items-batch-update-update`  -  Update multiple line items using their internal IDs or unique property values.
- `hubspot-cli hubspot-line-items-crm post-v3-objects-line-items-batch-upsert-upsert`  -  Create or update records identified by a unique property value as specified by the `idProperty` query param.
- `hubspot-cli hubspot-line-items-crm post-v3-objects-line-items-create`  -  Create a line item with the given properties and return a copy of the object, including the ID.
- `hubspot-cli hubspot-line-items-crm post-v3-objects-line-items-search-do-search`  -  Execute a search for line items based on filters, properties, and sorting options provided in the request body.

**hubspot-lists-crm**  -  Manage hubspot lists crm

- `hubspot-cli hubspot-lists-crm delete-v3-lists-folders-folder-id-v3-lists-folders-folder-id`  -  Delete v3 lists folders folder id v3 lists folders folder id
- `hubspot-cli hubspot-lists-crm delete-v3-lists-list-id-memberships-v3-lists-list-id-memberships`  -  Delete v3 lists list id memberships v3 lists list id memberships
- `hubspot-cli hubspot-lists-crm delete-v3-lists-list-id-schedule-conversion-v3-lists-list-id-schedule-conversion`  -  Delete v3 lists list id schedule conversion v3 lists list id schedule conversion
- `hubspot-cli hubspot-lists-crm delete-v3-lists-list-id-v3-lists-list-id`  -  Delete v3 lists list id v3 lists list id
- `hubspot-cli hubspot-lists-crm get-v3-lists-folders-v3-lists-folders`  -  Get v3 lists folders v3 lists folders
- `hubspot-cli hubspot-lists-crm get-v3-lists-idmapping-v3-lists-idmapping`  -  Get v3 lists idmapping v3 lists idmapping
- `hubspot-cli hubspot-lists-crm get-v3-lists-list-id-memberships-join-order-v3-lists-list-id-memberships-join-order`  -  Get v3 lists list id memberships join order v3 lists list id memberships join order
- `hubspot-cli hubspot-lists-crm get-v3-lists-list-id-memberships-v3-lists-list-id-memberships`  -  Get v3 lists list id memberships v3 lists list id memberships
- `hubspot-cli hubspot-lists-crm get-v3-lists-list-id-schedule-conversion-v3-lists-list-id-schedule-conversion`  -  Get v3 lists list id schedule conversion v3 lists list id schedule conversion
- `hubspot-cli hubspot-lists-crm get-v3-lists-list-id-size-and-edits-history-between-v3-lists-list-id-size-and-edits-history-between`  -  Get v3 lists list id size and edits history between v3 lists list id size and edits history between
- `hubspot-cli hubspot-lists-crm get-v3-lists-list-id-v3-lists-list-id`  -  Get v3 lists list id v3 lists list id
- `hubspot-cli hubspot-lists-crm get-v3-lists-object-type-id-object-type-id-name-list-name-v3-lists-object-type-id-object-type-id-name-list-name`  -  Retrieve a specific list by its name and object type ID.
- `hubspot-cli hubspot-lists-crm get-v3-lists-records-object-type-id-record-id-memberships-v3-lists-records-object-type-id-record-id-memberships`  -  Get v3 lists records object type id record id memberships v3 lists records object type id record id memberships
- `hubspot-cli hubspot-lists-crm get-v3-lists-v3-lists`  -  Get v3 lists v3 lists
- `hubspot-cli hubspot-lists-crm post-v3-lists-folders-v3-lists-folders`  -  Post v3 lists folders v3 lists folders
- `hubspot-cli hubspot-lists-crm post-v3-lists-idmapping-v3-lists-idmapping`  -  Post v3 lists idmapping v3 lists idmapping
- `hubspot-cli hubspot-lists-crm post-v3-lists-records-memberships-batch-read-v3-lists-records-memberships-batch-read`  -  Post v3 lists records memberships batch read v3 lists records memberships batch read
- `hubspot-cli hubspot-lists-crm post-v3-lists-search-v3-lists-search`  -  Post v3 lists search v3 lists search
- `hubspot-cli hubspot-lists-crm post-v3-lists-v3-lists`  -  Post v3 lists v3 lists
- `hubspot-cli hubspot-lists-crm put-v3-lists-folders-folder-id-move-new-parent-folder-id-v3-lists-folders-folder-id-move-new-parent-folder-id`  -  Put v3 lists folders folder id move new parent folder id v3 lists folders folder id move new parent folder id
- `hubspot-cli hubspot-lists-crm put-v3-lists-folders-folder-id-rename-v3-lists-folders-folder-id-rename`  -  Put v3 lists folders folder id rename v3 lists folders folder id rename
- `hubspot-cli hubspot-lists-crm put-v3-lists-folders-move-list-v3-lists-folders-move-list`  -  Put v3 lists folders move list v3 lists folders move list
- `hubspot-cli hubspot-lists-crm put-v3-lists-list-id-memberships-add-and-remove-v3-lists-list-id-memberships-add-and-remove`  -  Put v3 lists list id memberships add and remove v3 lists list id memberships add and remove
- `hubspot-cli hubspot-lists-crm put-v3-lists-list-id-memberships-add-from-source-list-id-v3-lists-list-id-memberships-add-from-source-list-id`  -  Put v3 lists list id memberships add from source list id v3 lists list id memberships add from source list id
- `hubspot-cli hubspot-lists-crm put-v3-lists-list-id-memberships-add-v3-lists-list-id-memberships-add`  -  Put v3 lists list id memberships add v3 lists list id memberships add
- `hubspot-cli hubspot-lists-crm put-v3-lists-list-id-memberships-remove-v3-lists-list-id-memberships-remove`  -  Put v3 lists list id memberships remove v3 lists list id memberships remove
- `hubspot-cli hubspot-lists-crm put-v3-lists-list-id-restore-v3-lists-list-id-restore`  -  Put v3 lists list id restore v3 lists list id restore
- `hubspot-cli hubspot-lists-crm put-v3-lists-list-id-schedule-conversion-v3-lists-list-id-schedule-conversion`  -  Put v3 lists list id schedule conversion v3 lists list id schedule conversion
- `hubspot-cli hubspot-lists-crm put-v3-lists-list-id-update-list-filters-v3-lists-list-id-update-list-filters`  -  Put v3 lists list id update list filters v3 lists list id update list filters
- `hubspot-cli hubspot-lists-crm put-v3-lists-list-id-update-list-name-v3-lists-list-id-update-list-name`  -  Put v3 lists list id update list name v3 lists list id update list name

**hubspot-meetings-crm**  -  Manage hubspot meetings crm

- `hubspot-cli hubspot-meetings-crm delete-v3-objects-meetings-meeting-id-archive`  -  Move an Object identified by `{meetingId}` to the recycling bin.
- `hubspot-cli hubspot-meetings-crm get-v3-objects-meetings-get-page`  -  Read a page of meetings. Control what is returned via the `properties` query param.
- `hubspot-cli hubspot-meetings-crm get-v3-objects-meetings-meeting-id-get-by-id`  -  Read an Object identified by `{meetingId}`.
- `hubspot-cli hubspot-meetings-crm patch-v3-objects-meetings-meeting-id-update`  -  Perform a partial update of an Object identified by `{meetingId}`or optionally a unique property value as specified by
- `hubspot-cli hubspot-meetings-crm post-v3-objects-meetings-batch-archive-archive`  -  Archive a batch of meetings by ID
- `hubspot-cli hubspot-meetings-crm post-v3-objects-meetings-batch-create-create`  -  Create a batch of meetings
- `hubspot-cli hubspot-meetings-crm post-v3-objects-meetings-batch-read-read`  -  Retrieve records by record ID or include the `idProperty` parameter to retrieve records by a custom unique value
- `hubspot-cli hubspot-meetings-crm post-v3-objects-meetings-batch-update-update`  -  Update a batch of meetings by internal ID, or unique property values
- `hubspot-cli hubspot-meetings-crm post-v3-objects-meetings-batch-upsert-upsert`  -  Create or update records identified by a unique property value as specified by the `idProperty` query param.
- `hubspot-cli hubspot-meetings-crm post-v3-objects-meetings-create`  -  Create a meeting with the given properties and return a copy of the object, including the ID.
- `hubspot-cli hubspot-meetings-crm post-v3-objects-meetings-search-do-search`  -  Post v3 objects meetings search do search

**hubspot-notes-crm**  -  Manage hubspot notes crm

- `hubspot-cli hubspot-notes-crm delete-v3-objects-notes-note-id-archive`  -  Move an Object identified by `{noteId}` to the recycling bin.
- `hubspot-cli hubspot-notes-crm get-v3-objects-notes-get-page`  -  Read a page of notes. Control what is returned via the `properties` query param.
- `hubspot-cli hubspot-notes-crm get-v3-objects-notes-note-id-get-by-id`  -  Read an Object identified by `{noteId}`.
- `hubspot-cli hubspot-notes-crm patch-v3-objects-notes-note-id-update`  -  Perform a partial update of an Object identified by `{noteId}`or optionally a unique property value as specified by the
- `hubspot-cli hubspot-notes-crm post-v3-objects-notes-batch-archive-archive`  -  Archive multiple notes by their IDs in a single request.
- `hubspot-cli hubspot-notes-crm post-v3-objects-notes-batch-create-create`  -  Create multiple notes in a single request by providing the necessary properties for each note.
- `hubspot-cli hubspot-notes-crm post-v3-objects-notes-batch-read-read`  -  Retrieve records by record ID or include the `idProperty` parameter to retrieve records by a custom unique value
- `hubspot-cli hubspot-notes-crm post-v3-objects-notes-batch-update-update`  -  Update multiple notes using their internal IDs or unique property values.
- `hubspot-cli hubspot-notes-crm post-v3-objects-notes-batch-upsert-upsert`  -  Create or update records identified by a unique property value as specified by the `idProperty` query param.
- `hubspot-cli hubspot-notes-crm post-v3-objects-notes-create`  -  Create a note with the given properties and return a copy of the object, including the ID.
- `hubspot-cli hubspot-notes-crm post-v3-objects-notes-search-do-search`  -  Execute a search for notes using filters, sorting options, and other query parameters to refine the results.

**hubspot-objects-crm**  -  Manage hubspot objects crm

- `hubspot-cli hubspot-objects-crm delete-v3-objects-object-type-object-id-archive`  -  Move an Object identified by `{objectId}` to the recycling bin.
- `hubspot-cli hubspot-objects-crm get-v3-objects-object-type-get-page`  -  Read a page of objects. Control what is returned via the `properties` query param.
- `hubspot-cli hubspot-objects-crm get-v3-objects-object-type-object-id-get-by-id`  -  Read an Object identified by `{objectId}`.
- `hubspot-cli hubspot-objects-crm patch-v3-objects-object-type-object-id-update`  -  Perform a partial update of an Object identified by `{objectId}`or optionally a unique property value as specified by
- `hubspot-cli hubspot-objects-crm post-v3-objects-object-type-create`  -  Create a CRM object with the given properties and return a copy of the object, including the ID.

**hubspot-owners-crm**  -  Manage hubspot owners crm

- `hubspot-cli hubspot-owners-crm get-v3-owners-owner-id-get-by-id`  -  Retrieve details of a specific owner using either their 'id' or 'userId'.
- `hubspot-cli hubspot-owners-crm get-v3-owners-v3-owners`  -  Get v3 owners v3 owners

**hubspot-pipelines-crm**  -  Manage hubspot pipelines crm

- `hubspot-cli hubspot-pipelines-crm delete-v3-pipelines-object-type-pipeline-id-archive`  -  Delete a pipeline
- `hubspot-cli hubspot-pipelines-crm delete-v3-pipelines-object-type-pipeline-id-stages-stage-id-archive`  -  Delete a pipeline stage
- `hubspot-cli hubspot-pipelines-crm get-v3-pipelines-object-type-get-all`  -  Return all pipelines for the object type specified by `{objectType}`.
- `hubspot-cli hubspot-pipelines-crm get-v3-pipelines-object-type-pipeline-id-audit-get-audit`  -  Return a reverse chronological list of all mutations that have occurred on the pipeline identified by `{pipelineId}`.
- `hubspot-cli hubspot-pipelines-crm get-v3-pipelines-object-type-pipeline-id-get-by-id`  -  Return a single pipeline object identified by its unique `{pipelineId}`.
- `hubspot-cli hubspot-pipelines-crm get-v3-pipelines-object-type-pipeline-id-stages-get-all`  -  Return all the stages associated with the pipeline identified by `{pipelineId}`.
- `hubspot-cli hubspot-pipelines-crm get-v3-pipelines-object-type-pipeline-id-stages-stage-id-audit-get-audit`  -  Return a reverse chronological list of all mutations that have occurred on the pipeline stage identified by `{stageId}`.
- `hubspot-cli hubspot-pipelines-crm get-v3-pipelines-object-type-pipeline-id-stages-stage-id-get-by-id`  -  Return a pipeline stage by ID
- `hubspot-cli hubspot-pipelines-crm patch-v3-pipelines-object-type-pipeline-id-stages-stage-id-update`  -  Patch v3 pipelines object type pipeline id stages stage id update
- `hubspot-cli hubspot-pipelines-crm patch-v3-pipelines-object-type-pipeline-id-update`  -  Perform a partial update of the pipeline identified by `{pipelineId}`.
- `hubspot-cli hubspot-pipelines-crm post-v3-pipelines-object-type-create`  -  Create a new pipeline with the provided property values.
- `hubspot-cli hubspot-pipelines-crm post-v3-pipelines-object-type-pipeline-id-stages-create`  -  Create a pipeline stage
- `hubspot-cli hubspot-pipelines-crm put-v3-pipelines-object-type-pipeline-id-replace`  -  Replace a pipeline
- `hubspot-cli hubspot-pipelines-crm put-v3-pipelines-object-type-pipeline-id-stages-stage-id-replace`  -  Replace all the properties of an existing pipeline stage with the values provided.

**hubspot-products-crm**  -  Manage hubspot products crm

- `hubspot-cli hubspot-products-crm delete-v3-objects-products-product-id-archive`  -  Move an Object identified by `{productId}` to the recycling bin.
- `hubspot-cli hubspot-products-crm get-v3-objects-products-get-page`  -  Read a page of products. Control what is returned via the `properties` query param.
- `hubspot-cli hubspot-products-crm get-v3-objects-products-product-id-get-by-id`  -  Read an Object identified by `{productId}`.
- `hubspot-cli hubspot-products-crm patch-v3-objects-products-product-id-update`  -  Perform a partial update of an Object identified by `{productId}`or optionally a unique property value as specified by
- `hubspot-cli hubspot-products-crm post-v3-objects-products-batch-archive-archive`  -  Archive multiple products at once by providing their IDs.
- `hubspot-cli hubspot-products-crm post-v3-objects-products-batch-create-create`  -  Create multiple products in a single request by specifying their properties
- `hubspot-cli hubspot-products-crm post-v3-objects-products-batch-read-read`  -  Retrieve records by record ID or include the `idProperty` parameter to retrieve records by a custom unique value
- `hubspot-cli hubspot-products-crm post-v3-objects-products-batch-update-update`  -  Update multiple products in a single request using their internal IDs or unique property values.
- `hubspot-cli hubspot-products-crm post-v3-objects-products-batch-upsert-upsert`  -  Create or update records identified by a unique property value as specified by the `idProperty` query param.
- `hubspot-cli hubspot-products-crm post-v3-objects-products-create`  -  Create a product with the given properties and return a copy of the object, including the ID.
- `hubspot-cli hubspot-products-crm post-v3-objects-products-search-do-search`  -  Execute a search for products based on defined filters, properties, and sorting options.

**hubspot-properties-batch**  -  Manage hubspot properties batch

- `hubspot-cli hubspot-properties-batch post-crm-v3-properties-object-type-archive-archive`  -  Archive a provided list of properties.
- `hubspot-cli hubspot-properties-batch post-crm-v3-properties-object-type-create-create`  -  Create a batch of properties using the same rules as when creating an individual property.
- `hubspot-cli hubspot-properties-batch post-crm-v3-properties-object-type-read-read`  -  Read a provided list of properties.

**hubspot-properties-crm**  -  Manage hubspot properties crm

- `hubspot-cli hubspot-properties-crm delete-v3-properties-object-type-property-name-archive`  -  Move a property identified by {propertyName} to the recycling bin.
- `hubspot-cli hubspot-properties-crm get-v3-properties-object-type-get-all`  -  Read all existing properties for the specified object type and HubSpot account.
- `hubspot-cli hubspot-properties-crm get-v3-properties-object-type-property-name-get-by-name`  -  Read a property identified by {propertyName}.
- `hubspot-cli hubspot-properties-crm patch-v3-properties-object-type-property-name-update`  -  Perform a partial update of a property identified by { propertyName }. Provided fields will be overwritten.
- `hubspot-cli hubspot-properties-crm post-v3-properties-object-type-create`  -  Create and return a copy of a new property for the specified object type.

**hubspot-quotes-crm**  -  Manage hubspot quotes crm

- `hubspot-cli hubspot-quotes-crm delete-v3-objects-quotes-quote-id-archive`  -  Move an Object identified by `{quoteId}` to the recycling bin.
- `hubspot-cli hubspot-quotes-crm get-v3-objects-quotes-get-page`  -  Read a page of quotes. Control what is returned via the `properties` query param.
- `hubspot-cli hubspot-quotes-crm get-v3-objects-quotes-quote-id-get-by-id`  -  Read an Object identified by `{quoteId}`.
- `hubspot-cli hubspot-quotes-crm patch-v3-objects-quotes-quote-id-update`  -  Perform a partial update of an Object identified by `{quoteId}`or optionally a unique property value as specified by
- `hubspot-cli hubspot-quotes-crm post-v3-objects-quotes-batch-archive-archive`  -  Archive multiple quotes by their IDs in a single request, effectively moving them to the recycling bin.
- `hubspot-cli hubspot-quotes-crm post-v3-objects-quotes-batch-create-create`  -  Create multiple quotes in a single request by providing a batch of quote objects
- `hubspot-cli hubspot-quotes-crm post-v3-objects-quotes-batch-read-read`  -  Retrieve records by record ID or include the `idProperty` parameter to retrieve records by a custom unique value
- `hubspot-cli hubspot-quotes-crm post-v3-objects-quotes-batch-update-update`  -  Update multiple quotes using their internal IDs or unique property values.
- `hubspot-cli hubspot-quotes-crm post-v3-objects-quotes-batch-upsert-upsert`  -  Create or update records identified by a unique property value as specified by the `idProperty` query param.
- `hubspot-cli hubspot-quotes-crm post-v3-objects-quotes-create`  -  Create a quote with the given properties and return a copy of the object, including the ID.
- `hubspot-cli hubspot-quotes-crm post-v3-objects-quotes-search-do-search`  -  Execute a search for quotes based on the criteria defined in the request body, such as filters, properties

**hubspot-tasks-crm**  -  Manage hubspot tasks crm

- `hubspot-cli hubspot-tasks-crm delete-v3-objects-tasks-task-id-archive`  -  Move an Object identified by `{taskId}` to the recycling bin.
- `hubspot-cli hubspot-tasks-crm get-v3-objects-tasks-get-page`  -  Read a page of tasks. Control what is returned via the `properties` query param.
- `hubspot-cli hubspot-tasks-crm get-v3-objects-tasks-task-id-get-by-id`  -  Read an Object identified by `{taskId}`.
- `hubspot-cli hubspot-tasks-crm patch-v3-objects-tasks-task-id-update`  -  Perform a partial update of an Object identified by `{taskId}`or optionally a unique property value as specified by the
- `hubspot-cli hubspot-tasks-crm post-v3-objects-tasks-batch-archive-archive`  -  Archive a batch of tasks by their IDs, moving them to the recycling bin.
- `hubspot-cli hubspot-tasks-crm post-v3-objects-tasks-batch-create-create`  -  Create multiple tasks in a single request by providing a batch of task properties and associations.
- `hubspot-cli hubspot-tasks-crm post-v3-objects-tasks-batch-read-read`  -  Retrieve records by record ID or include the `idProperty` parameter to retrieve records by a custom unique value
- `hubspot-cli hubspot-tasks-crm post-v3-objects-tasks-batch-update-update`  -  Update multiple tasks in a single request using their internal IDs or unique property values.
- `hubspot-cli hubspot-tasks-crm post-v3-objects-tasks-batch-upsert-upsert`  -  Create or update records identified by a unique property value as specified by the `idProperty` query param.
- `hubspot-cli hubspot-tasks-crm post-v3-objects-tasks-create`  -  Create a task with the given properties and return a copy of the object, including the ID.
- `hubspot-cli hubspot-tasks-crm post-v3-objects-tasks-search-do-search`  -  Execute a search for tasks based on the provided criteria, including filters, properties, and sorting options.

**hubspot-tickets-crm**  -  Manage hubspot tickets crm

- `hubspot-cli hubspot-tickets-crm delete-v3-objects-tickets-ticket-id-archive`  -  Move an Object identified by `{ticketId}` to the recycling bin.
- `hubspot-cli hubspot-tickets-crm get-v3-objects-tickets-get-page`  -  Read a page of tickets. Control what is returned via the `properties` query param.
- `hubspot-cli hubspot-tickets-crm get-v3-objects-tickets-ticket-id-get-by-id`  -  Read an Object identified by `{ticketId}`.
- `hubspot-cli hubspot-tickets-crm patch-v3-objects-tickets-ticket-id-update`  -  Perform a partial update of an Object identified by `{ticketId}`or optionally a unique property value as specified by
- `hubspot-cli hubspot-tickets-crm post-v3-objects-tickets-batch-archive-archive`  -  Delete a batch of tickets by ID. Deleted tickets can be restored within 90 days of deletion.
- `hubspot-cli hubspot-tickets-crm post-v3-objects-tickets-batch-create-create`  -  Create a batch of tickets.
- `hubspot-cli hubspot-tickets-crm post-v3-objects-tickets-batch-read-read`  -  Retrieve a batch of tickets by ID (`ticketId`) or unique property value (`idProperty`).
- `hubspot-cli hubspot-tickets-crm post-v3-objects-tickets-batch-update-update`  -  Update a batch of tickets by ID (`ticketId`) or unique property value (`idProperty`).
- `hubspot-cli hubspot-tickets-crm post-v3-objects-tickets-batch-upsert-upsert`  -  Create or update records identified by a unique property value as specified by the `idProperty` query param.
- `hubspot-cli hubspot-tickets-crm post-v3-objects-tickets-create`  -  Create a ticket with the given properties and return a copy of the object, including the ID.
- `hubspot-cli hubspot-tickets-crm post-v3-objects-tickets-merge-merge`  -  Merge two tickets, combining them into one ticket record.
- `hubspot-cli hubspot-tickets-crm post-v3-objects-tickets-search-do-search`  -  Search for tickets by filtering on properties, searching through associations, and sorting results.

**objects_search**  -  Manage objects search

- `hubspot-cli objects-search <objectType>`  -  Post crm v3 objects object type search do search


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
hubspot-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes


### Monthly customer report  -  every meeting EVER scheduled in April

```bash
hubspot-cli sync --resources hubspot-meetings-crm --with-history hs_meeting_outcome,hs_meeting_title,hubspot_owner_id && hubspot-cli meetings status-report --status scheduled --month 2026-04 --csv > april-scheduled.csv
```

Two commands: refresh meetings with property history retained, then emit the canonical CSV the customer expects. Includes every meeting that was set to Scheduled at any point in April, even if it later flipped to No Show or Completed  -  only the property-history snapshot table can answer this.

### Investigate a single meeting's status history

```bash
hubspot-cli meetings ever-had --property hs_meeting_outcome --value Scheduled --from 2026-04-01 --to 2026-04-30 --agent --select id,property,value,timestamp,source_type --json
```

Find every meeting touching status Scheduled in April; agent-friendly dotted-path --select keeps only the columns the agent needs, --agent enables compact output.

### Today's nurture queue for me

```bash
hubspot-cli nurture queue --owner me --top 20 --agent
```

Ranked daily contact list with stale-days, deal $, and stage probability columns  -  drop straight into `/nurture today`.

### Cross-object timeline for a deal

```bash
hubspot-cli engagements of deal:1234567 --since 90d --json --select id,type,timestamp,subject,from_owner
```

Every call, email, meeting, note, and task touching the deal in the last 90 days  -  one query replaces five paginated API calls; --select keeps the agent context tight.

### Find every cold deal in my pipeline

```bash
hubspot-cli stale deals --days 21 --owner me --json --select id,name,amount,stage,idle_days
```

Open deals that have not had an engagement in 21 days, scoped to me, with only the fields an agent needs.

### Bulk-update lifecycle stage from a campaign CSV

```bash
hubspot-cli contacts bulk-update --from-csv titans.csv --map email=email,lifecyclestage=Stage --dry-run
```

Validate the CSV against the local properties schema before any write; drop --dry-run when the report is clean.

## Auth Setup

Authenticate with a HubSpot Private App access token (prefix `pat-…`). Create one at https://app.hubspot.com/private-apps with CRM read+write scopes, then export it as `HUBSPOT_ACCESS_TOKEN`. The token's scopes determine which commands work  -  `doctor` reports them.

Run `hubspot-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, sync, and `--deliver` paths:

- `--json`  -  one JSON document on stdout (sync progress events go to stderr)
- `--compact`  -  keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain`  -  tabular rows (collection envelopes unwrap to the row array)
- `--quiet`  -  one identity value per row, no envelope

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  hubspot-cli crm get-v4-objects-object-type-object-id-associations-to-object-type-get-page mock-value mock-value mock-value --agent --select paging,results
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

- Use `--home <dir>` for one invocation, or set `HUBSPOT_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `HUBSPOT_CONFIG_DIR`, `HUBSPOT_DATA_DIR`, `HUBSPOT_STATE_DIR`, `HUBSPOT_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `HUBSPOT_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `hubspot-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "hubspot": {
        "command": "hubspot-mcp",
        "env": {
          "HUBSPOT_HOME": "/srv/hubspot"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `HUBSPOT_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `HUBSPOT_HOME`, or `doctor` will not find credentials left under the former root.

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
hubspot-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "hubspot-cli learnings confirm 12"] }
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
       materially more, record the divergence via `hubspot-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `hubspot-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `hubspot-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
hubspot-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
hubspot-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
hubspot-cli teach-playbook \
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
hubspot-cli playbook amend \
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

`hubspot-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `HUBSPOT_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
hubspot-cli feedback "the --since flag is inclusive but docs say exclusive"
hubspot-cli feedback --stdin < notes.txt
hubspot-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `HUBSPOT_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `HUBSPOT_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
hubspot-cli profile save briefing --json
hubspot-cli --profile briefing crm get-v4-objects-object-type-object-id-associations-to-object-type-get-page mock-value mock-value mock-value
hubspot-cli profile list --json
hubspot-cli profile show briefing
hubspot-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `hubspot-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add hubspot-mcp -- hubspot-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which hubspot-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   hubspot-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `hubspot-cli <command> --help`.
