---
name: pipedrive
description: "Full Pipedrive CRUD plus a local SQLite pipeline copy: stale deals, forecasts, aging, dupes, rep leaderboards. Trigger phrases: `which deals are going stale in pipedrive`, `what's my weighted pipeline forecast`, `who do I need to follow up with today`, `find duplicate contacts in pipedrive`, `show the sales rep leaderboard`, `use pipedrive`, `run pipedrive`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Pipedrive"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - pipedrive-cli
    install:
      - kind: go
        bins: [pipedrive-cli]
        module: github.com/mvanhorn/printing-press-library/library/sales-and-crm/pipedrive/cmd/pipedrive-cli
---

# Pipedrive  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `pipedrive-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install pipedrive --cli-only
   ```
2. Verify: `pipedrive-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/sales-and-crm/pipedrive/cmd/pipedrive-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Existing Pipedrive CLIs and MCP servers are thin mirrors of the REST API  -  none keep a local copy of your pipeline, so none can tell you which deals are going stale, what your weighted forecast is, who's aging in a stage, or where your duplicate contacts are. This CLI syncs the whole deal-person-org-activity graph into local SQLite with full-text search, then adds the cross-entity intelligence layer on top: stale, forecast, aging, digest, changes, dupes, and leaderboard. Plus an offline `search`, offline `analytics` (count and group-by over synced data), and agent-native `--json`/`--select`/`--csv` on every command.

## When to Use This CLI

Reach for this CLI when an agent or operator needs to reason about a Pipedrive pipeline as a dataset rather than poke individual records: morning triage of stale deals, weekly weighted forecasting and per-rep leaderboards, deduping contacts, scripting incremental change feeds, or running offline analytics over the synced CRM. It is the right tool whenever the question spans many deals, a time window, or several entities at once  -  the cases a single REST call can't answer.

## Anti-triggers

Do not use this CLI for:
- Fuzzy text lookup across many records to find an entity  -  use the 'search' command (or the API search endpoints), not 'who'.
- Live per-record reads that must reflect this second's CRM state  -  the novel commands read the local synced store; run 'sync' first or use the generated endpoint commands.
- Pipeline analytics before a first 'sync --full'  -  every local-join command (stale, forecast, aging, digest, changes, dupes, leaderboard, next-activity, lost, who) returns empty counts on an unsynced store.
- Sending email or managing Pipedrive automations/workflows  -  not part of this CLI's surface.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Pipeline intelligence (local-join)
- **`stale`**  -  List open deals nobody has touched in N days, ranked by the dollar value at risk.

  _Reach for this every morning to catch the deals silently dying before they're lost  -  the highest-leverage read in the whole CLI._

  ```bash
  pipedrive-cli stale --quiet-days 14 --agent
  ```
- **`forecast`**  -  Weighted pipeline value (deal value times stage probability) by pipeline, plus what is expected to close this period.

  _Use this for weekly pipeline review instead of exporting CSVs and rebuilding the weighted-value math by hand._

  ```bash
  pipedrive-cli forecast --period this-quarter --agent
  ```
- **`aging`**  -  Show which deals are stuck in a stage longer than that stage's typical dwell time.

  _Reach for this to find the bottleneck stage and the specific deals rotting in it before a pipeline review._

  ```bash
  pipedrive-cli aging --agent
  ```
- **`leaderboard`**  -  Per-rep open/won/lost counts, weighted pipeline, won value, and activity count over a window.

  _Use this for team reviews to rank reps by real contribution without touching a spreadsheet._

  ```bash
  pipedrive-cli leaderboard --by won-value --window 90d --agent
  ```
- **`next-activity`**  -  List open deals with no future activity scheduled, ranked by the value at risk of falling through the cracks.

  _Run this daily to catch the deals you forgot to plan a next step for  -  distinct from stale, which catches deals you have not touched._

  ```bash
  pipedrive-cli next-activity --missing --agent
  ```
- **`lost`**  -  All deals marked Lost in a recent window with their person, org, owner, and lost reason  -  ready for re-enrollment.

  _Use this to build a re-engagement campaign from deals lost in a window, with contact info and lost reasons already joined in._

  ```bash
  pipedrive-cli lost --since 180d --agent
  ```
- **`who`**  -  One-shot card for a person: their org, open deals and value, last and next activity, and recent notes  -  joined from the local store.

  _Reach for this before a call to get the full relationship picture of one contact in a single command instead of five lookups._

  ```bash
  pipedrive-cli who "Jane Smith" --agent
  ```

### Standup & change tracking
- **`digest`**  -  One-shot standup rollup: new deals since yesterday, deals gone stale today, overdue and due-today activities, and deals won or lost since.

  _Run this first thing  -  it answers 'what changed and who do I need to call today' in one command._

  ```bash
  pipedrive-cli digest --for-me --agent
  ```
- **`changes`**  -  Everything whose update time moved since a timestamp, grouped by entity.

  _Use this in scripts to process only what changed since the last run instead of re-paging the whole API._

  ```bash
  pipedrive-cli changes --since 24h --agent
  ```

### Data hygiene
- **`dupes`**  -  Find likely-duplicate persons or organizations by normalized name, email, and phone.

  _Reach for this to clean up the duplicate contacts that integrations quietly create._

  ```bash
  pipedrive-cli dupes --entity persons --agent
  ```

## Command Reference

**activities**  -  Activities are appointments/tasks/events on a calendar that can be associated with a deal, a lead, a person and an organization. Activities can be of different type (such as call, meeting, lunch or a custom type - see ActivityTypes object) and can be assigned to a particular user. Note that activities can also be created without a specific date/time.

- `pipedrive-cli activities add`  -  Adds a new activitie. (Restored core v1 create endpoint.)
- `pipedrive-cli activities delete`  -  Marks a activitie as deleted. (Restored core v1 delete endpoint.)
- `pipedrive-cli activities get`  -  Returns the details of a specific activitie. (Restored core v1 detail endpoint.)
- `pipedrive-cli activities get-all`  -  Returns all activities. (Restored core v1 list endpoint.)
- `pipedrive-cli activities update`  -  Updates the properties of a activitie. (Restored core v1 update endpoint.)

**activity-fields**  -  Activity fields represent different fields that an activity has.

- `pipedrive-cli activity-fields`  -  Returns all activity fields.

**activity-types**  -  Activity types represent different kinds of activities that can be stored. Each activity type is presented to the user with an icon and a name. Additionally, a color can be defined (not implemented in the Pipedrive app as of today). Activity types are linked to activities via `ActivityType.key_string = Activity.type`. The `key_string` will be generated by the API based on the given name of the activity type upon creation, and cannot be changed. Activity types should be presented to the user in an ordered manner, using the `ActivityType.order_nr` value.

- `pipedrive-cli activity-types add`  -  Adds a new activity type.
- `pipedrive-cli activity-types delete`  -  Marks an activity type as deleted.
- `pipedrive-cli activity-types get`  -  Returns all activity types.
- `pipedrive-cli activity-types update`  -  Updates an activity type.

**billing**  -  Billing is responsible for handling your subscriptions, payments, plans and add-ons.

- `pipedrive-cli billing`  -  Returns the add-ons for a single company.

**call-logs**  -  Call logs describe the outcome of a phone call managed by an integrated provider. Since these logs are also considered activities, they can be associated with a deal or a lead, a person and/or an organization. Call logs do differ from other activities, as they only receive the information needed to describe the phone call.

- `pipedrive-cli call-logs add`  -  Adds a new call log.
- `pipedrive-cli call-logs delete`  -  Deletes a call log. If there is an audio recording attached to it, it will also be deleted.
- `pipedrive-cli call-logs get`  -  Returns details of a specific call log.
- `pipedrive-cli call-logs get-user`  -  Returns all call logs assigned to a particular user.

**channels**  -  Channels API allows you to integrate your existing messaging channels into Pipedrive through [Messaging app extension](https://pipedrive.readme.io/docs/messaging-app-extension). It enables you to manage and interact with the channel’s conversations, participants and messages inside Pipedrive Messaging inbox: get the historical conversation, receive and send new messages. These endpoints are accessible only through **Messengers integration** OAuth scope together with Messaging manifest in building the [Messaging app extension](https://pipedrive.readme.io/docs/messaging-app-extension).

- `pipedrive-cli channels add`  -  Adds a new messaging channel, only admins are able to register new channels.
- `pipedrive-cli channels delete`  -  Deletes an existing messenger’s channel and all related entities (conversations and messages).
- `pipedrive-cli channels receive-message`  -  Adds a message to a conversation.

**currencies**  -  Supported currencies which can be used to represent the monetary value of a deal, or a value of any monetary type custom field. The `Currency.code` field must be used to point to a currency. `Currency.code` is the ISO-4217 format currency code for non-custom currencies. You can differentiate custom and non-custom currencies using the `is_custom_flag` property. For custom currencies, it is intended that the formatted sums are displayed in the UI using the following format: [sum][non-breaking space character][currency.symbol], for example: 500 users. Custom currencies cannot be added or removed via the API yet  -  rather the admin users of the account must configure them from the Pipedrive app.

- `pipedrive-cli currencies`  -  Returns all supported currencies in given account which should be used when saving monetary values with other objects.

**deal-fields**  -  Deal fields represent the near-complete schema for a deal in the context of the company of the authorized user. Each company can have a different schema for their deals, with various custom fields. In the context of using deal fields as a schema for defining the data fields of a deal, it must be kept in mind that some types of custom fields can have additional data fields which are not separate deal fields per se. Such is the case with monetary, daterange and timerange fields – each of these fields will have one additional data field in addition to the one presented in the context of deal fields. For example, if there is a monetary field with the key `ffk9s9` stored on the account, `ffk9s9` would hold the numeric value of the field, and `ffk9s9_currency` would hold the ISO currency code that goes along with the numeric value. To find out which data fields are available, fetch one deal and list its keys.

- `pipedrive-cli deal-fields add`  -  Adds a new deal field. For more information, see the tutorial for adding a new custom field .
- `pipedrive-cli deal-fields delete`  -  Marks multiple deal fields as deleted.
- `pipedrive-cli deal-fields delete-dealfields`  -  Marks a field as deleted. For more information, see the tutorial for deleting a custom field .
- `pipedrive-cli deal-fields get`  -  Returns data about all deal fields.
- `pipedrive-cli deal-fields get-dealfields`  -  Returns data about a specific deal field.
- `pipedrive-cli deal-fields update`  -  Updates a deal field. For more information, see the tutorial for updating custom fields' values .

**deals**  -  Deals represent ongoing, lost or won sales to an organization or to a person. Each deal has a monetary value and must be placed in a stage. Deals can be owned by a user, and followed by one or many users. Each deal consists of standard data fields but can also contain a number of custom fields. The custom fields can be recognized by long hashes as keys. These hashes can be mapped against `DealField.key`. The corresponding label for each such custom field can be obtained from `DealField.name`.

- `pipedrive-cli deals add`  -  Adds a new deal. (Restored core v1 create endpoint.)
- `pipedrive-cli deals delete`  -  Marks a deal as deleted. (Restored core v1 delete endpoint.)
- `pipedrive-cli deals get`  -  Returns the details of a specific deal. (Restored core v1 detail endpoint.)
- `pipedrive-cli deals get-all`  -  Returns all deals. (Restored core v1 list endpoint.)
- `pipedrive-cli deals get-archived`  -  Returns all archived deals.
- `pipedrive-cli deals get-archived-summary`  -  Returns a summary of all archived deals.
- `pipedrive-cli deals get-archived-timeline`  -  Returns archived open and won deals, grouped by a defined interval of time set in a date-type dealField (`field_key`)
- `pipedrive-cli deals get-summary`  -  Returns a summary of all not archived deals.
- `pipedrive-cli deals get-timeline`  -  Returns not archived open and won deals
- `pipedrive-cli deals update`  -  Updates the properties of a deal. (Restored core v1 update endpoint.)

**files**  -  Files are documents of any kind (images, spreadsheets, text files, etc.) that are uploaded to Pipedrive, and usually associated with a particular deal, person, organization, product, note or activity. Remote files can only be associated with a particular deal, person or organization. Note that the API currently does not support downloading files although it lets you retrieve a file’s meta-info along with a URL which can be used to download the file by using a standard HTTP GET request.

- `pipedrive-cli files add`  -  Lets you upload a file and associate it with a deal, person, organization, activity, product or lead.
- `pipedrive-cli files add-and-link-it`  -  Creates a new empty file in the remote location (`googledrive`) that will be linked to the item you supply.
- `pipedrive-cli files delete`  -  Marks a file as deleted. After 30 days, the file will be permanently deleted.
- `pipedrive-cli files get`  -  Returns data about all files.
- `pipedrive-cli files get-id`  -  Returns data about a specific file.
- `pipedrive-cli files link-to-item`  -  Links an existing remote file (`googledrive`) to the item you supply.
- `pipedrive-cli files update`  -  Updates the properties of a file.

**filters**  -  Each filter is essentially a set of data validation conditions. A filter of the same kind can be applied when fetching a list of deals, leads, persons, organizations or products in the context of a pipeline. Filters are limited to a maximum of 16 conditions. When applied, only items matching the conditions of the filter are returned. Detailed definitions of filter conditions and additional functionality is not yet available.

- `pipedrive-cli filters add`  -  Adds a new filter, returns the ID upon success.
- `pipedrive-cli filters delete`  -  Marks multiple filters as deleted.
- `pipedrive-cli filters delete-id`  -  Marks a filter as deleted.
- `pipedrive-cli filters get`  -  Returns data about all filters.
- `pipedrive-cli filters get-helpers`  -  Returns all supported filter helpers.
- `pipedrive-cli filters get-id`  -  Returns data about a specific filter. Note that this also returns the condition lines of the filter.
- `pipedrive-cli filters update`  -  Updates an existing filter.

**goals**  -  Goals help your team meet your sales targets. There are three types of goals - company, team and user.

- `pipedrive-cli goals add`  -  Adds a new goal. Along with adding a new goal, a report is created to track the progress of your goal.
- `pipedrive-cli goals delete`  -  Marks a goal as deleted.
- `pipedrive-cli goals get`  -  Returns data about goals based on criteria.
- `pipedrive-cli goals update`  -  Updates an existing goal.

**lead-fields**  -  Lead fields represent the near-complete schema for a lead in the context of the company of the authorized user. Each company can have a different schema for their leads, with various custom fields. In the context of using lead fields as a schema for defining the data fields of a lead, it must be kept in mind that some types of custom fields can have additional data fields which are not separate lead fields per se. Such is the case with monetary, daterange and timerange fields – each of these fields will have one additional data field in addition to the one presented in the context of lead fields. For example, if there is a monetary field with the key `ffk9s9` stored on the account, `ffk9s9` would hold the numeric value of the field, and `ffk9s9_currency` would hold the ISO currency code that goes along with the numeric value. To find out which data fields are available, fetch one lead and list its keys.

- `pipedrive-cli lead-fields`  -  Returns data about all lead fields.

**lead-labels**  -  Lead labels allow you to visually categorize your leads. There are three default lead labels: hot, cold, and warm, but you can add as many new custom labels as you want.

- `pipedrive-cli lead-labels add`  -  Creates a lead label.
- `pipedrive-cli lead-labels delete`  -  Deletes a specific lead label.
- `pipedrive-cli lead-labels get`  -  Returns details of all lead labels. This endpoint does not support pagination and all labels are always returned.
- `pipedrive-cli lead-labels update`  -  Updates one or more properties of a lead label. Only properties included in the request will be updated.

**lead-sources**  -  A lead source indicates where your lead came from. Currently, these are the possible lead sources: `Manually created`, `Deal`, `Web forms`, `Prospector`, `Leadbooster`, `Live chat`, `Import`, `Website visitors`, `Workflow automation`, and `API`. Lead sources are pre-defined and cannot be edited. Please note that leads sourced from the Chatbot feature are assigned the value `Leadbooster`. Please also note that this list is not final and new sources may be added as needed.

- `pipedrive-cli lead-sources`  -  Returns all lead sources. Please note that the list of lead sources is fixed, it cannot be modified.

**leads**  -  Leads are potential deals stored in Leads Inbox before they are archived or converted to a deal. Each lead needs to be named (using the `title` field) and be linked to a person or an organization. In addition to that, a lead can contain most of the fields a deal can (such as `value` or `expected_close_date`).

- `pipedrive-cli leads add`  -  Creates a lead. A lead always has to be linked to a person or an organization or both.
- `pipedrive-cli leads delete`  -  Deletes a specific lead.
- `pipedrive-cli leads get`  -  Returns multiple not archived leads. Leads are sorted by the time they were created, from oldest to newest.
- `pipedrive-cli leads get-archived`  -  Returns multiple archived leads. Leads are sorted by the time they were created, from oldest to newest.
- `pipedrive-cli leads get-id`  -  Returns details of a specific lead.
- `pipedrive-cli leads search`  -  Searches all leads by title, notes and/or custom fields.
- `pipedrive-cli leads update`  -  Updates one or more properties of a lead. Only properties included in the request will be updated.

**legacy-teams**  -  Legacy teams allow you to form groups of users withing the organization for more efficient management. Previously Legacy Teams were called Teams and occupied the `v1/teams*` path. They're being deprecated because we are preparing for an upgraded version of the Teams API, which requires migrating the current functionality to a new path URL `v1/legacyTeams*`. The functionality and [OAuth scopes](https://pipedrive.readme.io/docs/marketplace-scopes-and-permissions-explanations) of all the Teams API endpoints will remain the same.

- `pipedrive-cli legacy-teams add-team`  -  Adds a new team to the company and returns the created object.
- `pipedrive-cli legacy-teams get-team`  -  Returns data about a specific team.
- `pipedrive-cli legacy-teams get-teams`  -  Returns data about teams within the company.
- `pipedrive-cli legacy-teams get-user-teams`  -  Returns data about all teams which have the specified user as a member.
- `pipedrive-cli legacy-teams update-team`  -  Updates an existing team and returns the updated object.

**mailbox**  -  Mailbox was designed to be the email control hub inside Pipedrive. Pipedrive supports all major providers (including Gmail, Outlook and also custom IMAP/SMTP). There are 2 options for syncing user emails: 2-way sync: Mail Connection is established with the mail provider (example Gmail). There can be only 1 active Mail Connection per user in company. 1-way sync: SmartBCC feature which stores the copies of email messages to Pipedrive by adding the SmartBCC specific address to mail recipients.

- `pipedrive-cli mailbox delete-mail-thread`  -  Marks a mail thread as deleted.
- `pipedrive-cli mailbox get-mail-message`  -  Returns data about a specific mail message.
- `pipedrive-cli mailbox get-mail-thread`  -  Returns a specific mail thread.
- `pipedrive-cli mailbox get-mail-thread-messages`  -  Returns all the mail messages inside a specified mail thread.
- `pipedrive-cli mailbox get-mail-threads`  -  Returns mail threads in a specified folder ordered by the most recent message within.
- `pipedrive-cli mailbox update-mail-thread-details`  -  Updates the properties of a mail thread.

**meetings**  -  Meetings API allows integrating video calling apps into Pipedrive through [Video Calling App extension](https://pipedrive.readme.io/docs/video-calling-app-extension). It enables you to manage and interact with your video calls and meetings inside Pipedrive. These endpoints are accessible only through apps with video calls integration [OAuth scope](https://pipedrive.readme.io/docs/marketplace-scopes-and-permissions-explanations).

- `pipedrive-cli meetings delete-user-provider-link`  -  A video calling provider must call this endpoint to remove the link between a user and the installed video calling app.
- `pipedrive-cli meetings save-user-provider-link`  -  A video calling provider must call this endpoint after a user has installed the video calling app so that the new

**note-fields**  -  Note fields represent different fields that a note has.

- `pipedrive-cli note-fields`  -  Returns data about all note fields.

**notes**  -  Notes are pieces of textual (HTML-formatted) information that can be attached to deals, persons and organizations. Notes are usually displayed in the UI in chronological order – newest first – and in context with other updates regarding the item they are attached to. The maximum note size is approximately 100,000 characters (or 100KB per note).

- `pipedrive-cli notes add`  -  Adds a new note.
- `pipedrive-cli notes delete`  -  Deletes a specific note.
- `pipedrive-cli notes get`  -  Returns all notes.
- `pipedrive-cli notes get-id`  -  Returns details about a specific note.
- `pipedrive-cli notes update`  -  Updates a note.

**oauth**  -  Using OAuth 2.0 is necessary for developing apps that are available in the Pipedrive Marketplace. Authorization via OAuth 2.0 is a well-known and stable way to get fine-grained access to an API. To retrieve OAuth2 tokens you should send requests to the `https://oauth.pipedrive.com` domain. After registering the app, you must add the necessary server-side logic to your app to establish the OAuth flow. Please read more about authorization step on the [Pipedrive Developers page](https://pipedrive.readme.io/docs/marketplace-oauth-authorization).

- `pipedrive-cli oauth authorize`  -  Authorize a user by redirecting them to the Pipedrive OAuth authorization page and request their permissions to act on
- `pipedrive-cli oauth get-tokens`  -  After the customer has confirmed the app installation
- `pipedrive-cli oauth refresh-tokens`  -  The `access_token` has a lifetime.

**organization-fields**  -  Organization fields represent the near-complete schema for an organization in the context of the company of the authorized user. Each company can have a different schema for their organizations, with various custom fields. In the context of using organization fields as a schema for defining the data fields of an organization, it must be kept in mind that some types of custom fields can have additional data fields which are not separate organization fields per se. Such is the case with monetary, daterange and timerange fields – each of these fields will have one additional data field in addition to the one presented in the context of organization fields. For example, if there is a monetary field with the key `ffk9s9` stored on the account, `ffk9s9` would hold the numeric value of the field, and `ffk9s9_currency` would hold the ISO currency code that goes along with the numeric value. To find out which data fields are available, fetch one organization and list its keys.

- `pipedrive-cli organization-fields add`  -  Adds a new organization field. For more information, see the tutorial for adding a new custom field .
- `pipedrive-cli organization-fields delete`  -  Delete multiple organization fields in bulk
- `pipedrive-cli organization-fields delete-organizationfields`  -  Marks a field as deleted. For more information, see the tutorial for deleting a custom field .
- `pipedrive-cli organization-fields get`  -  Returns data about all organization fields.
- `pipedrive-cli organization-fields get-organizationfields`  -  Returns data about a specific organization field.
- `pipedrive-cli organization-fields update`  -  Updates an organization field. For more information, see the tutorial for updating custom fields' values .

**organization-relationships**  -  Organization relationships represent how different organizations are related to each other. The relationship can be hierarchical (parent-child companies) or lateral as defined by the `type` field - either `parent` or `related`.

- `pipedrive-cli organization-relationships add`  -  Creates and returns an organization relationship.
- `pipedrive-cli organization-relationships delete`  -  Deletes an organization relationship and returns the deleted ID.
- `pipedrive-cli organization-relationships get`  -  Gets all of the relationships for a supplied organization ID.
- `pipedrive-cli organization-relationships get-organizationrelationships`  -  Finds and returns an organization relationship from its ID.
- `pipedrive-cli organization-relationships update`  -  Updates and returns an organization relationship.

**organizations**  -  Organizations are companies and other kinds of organizations you are making deals with. Persons can be associated with organizations so that each organization can contain one or more persons.

- `pipedrive-cli organizations add`  -  Adds a new organization. (Restored core v1 create endpoint.)
- `pipedrive-cli organizations delete`  -  Marks a organization as deleted. (Restored core v1 delete endpoint.)
- `pipedrive-cli organizations get`  -  Returns the details of a specific organization. (Restored core v1 detail endpoint.)
- `pipedrive-cli organizations get-all`  -  Returns all organizations. (Restored core v1 list endpoint.)
- `pipedrive-cli organizations update`  -  Updates the properties of a organization. (Restored core v1 update endpoint.)

**permission-sets**  -  Permission sets define what users in the account can do: which actions they are allowed to perform and which features they can access. Permission sets are app-specific, where apps are large parts of functionality, e.g., sales app, which allows accessing sales data, global permissions, which oversee cross-product features (for example contacts, insights, products) or account settings, which provides access to billing, user management, company settings and security center. Some permission sets with types such as admin and regular are pre-created for the account, while other custom ones can be created by users (depending on the tier the account is on).

- `pipedrive-cli permission-sets get`  -  Returns data about all permission sets.
- `pipedrive-cli permission-sets get-permissionsets`  -  Returns data about a specific permission set.

**person-fields**  -  Person fields represent the near-complete schema for a person in the context of the company of the authorized user. Each company can have a different schema for their persons, with various custom fields. In the context of using person fields as a schema for defining the data fields of a person, it must be kept in mind that some types of custom fields can have additional data fields which are not separate person fields per se. Such is the case with monetary, daterange and timerange fields – each of these fields will have one additional data field in addition to the one presented in the context of person fields. For example, if there is a monetary field with the key `ffk9s9` stored on the account, `ffk9s9` would hold the numeric value of the field, and `ffk9s9_currency` would hold the ISO currency code that goes along with the numeric value. To find out which data fields are available, fetch one person and list its keys.

- `pipedrive-cli person-fields add`  -  Adds a new person field. For more information, see the tutorial for adding a new custom field .
- `pipedrive-cli person-fields delete`  -  Delete multiple person fields in bulk
- `pipedrive-cli person-fields delete-personfields`  -  Marks a field as deleted. For more information, see the tutorial for deleting a custom field .
- `pipedrive-cli person-fields get`  -  Returns data about all person fields. If a company uses the [Campaigns product](https://pipedrive.readme.
- `pipedrive-cli person-fields get-personfields`  -  Returns data about a specific person field.
- `pipedrive-cli person-fields update`  -  Updates a person field. For more information, see the tutorial for updating custom fields' values .

**persons**  -  Persons are your contacts, the customers you are doing deals with. Each person can belong to an organization. Persons should not be confused with users.

- `pipedrive-cli persons add`  -  Adds a new person. (Restored core v1 create endpoint.)
- `pipedrive-cli persons delete`  -  Marks a person as deleted. (Restored core v1 delete endpoint.)
- `pipedrive-cli persons get`  -  Returns the details of a specific person. (Restored core v1 detail endpoint.)
- `pipedrive-cli persons get-all`  -  Returns all persons. (Restored core v1 list endpoint.)
- `pipedrive-cli persons update`  -  Updates the properties of a person. (Restored core v1 update endpoint.)

**pipelines**  -  Pipelines are essentially ordered collections of stages.

- `pipedrive-cli pipelines get`  -  Returns the details of a specific pipeline. (Restored core v1 detail endpoint.)
- `pipedrive-cli pipelines get-all`  -  Returns all pipelines. (Restored core v1 list endpoint.)

**product-fields**  -  Product fields represent the near-complete schema for a product in the context of the company of the authorized user. Each company can have a different schema for their products, with various custom fields. In the context of using product fields as a schema for defining the data fields of a product, it must be kept in mind that some types of custom fields can have additional data fields which are not separate product fields per se. Such is the case with monetary, daterange and timerange fields – each of these fields will have one additional data field in addition to the one presented in the context of product fields. For example, if there is a monetary field with the key `ffk9s9` stored on the account, `ffk9s9` would hold the numeric value of the field, and `ffk9s9_currency` would hold the ISO currency code that goes along with the numeric value. To find out which data fields are available, fetch one product and list its keys.

- `pipedrive-cli product-fields add`  -  Adds a new product field. For more information, see the tutorial for adding a new custom field .
- `pipedrive-cli product-fields delete`  -  Delete multiple product fields in bulk
- `pipedrive-cli product-fields delete-productfields`  -  Marks a product field as deleted. For more information, see the tutorial for deleting a custom field .
- `pipedrive-cli product-fields get`  -  Returns data about all product fields.
- `pipedrive-cli product-fields get-productfields`  -  Returns data about a specific product field.
- `pipedrive-cli product-fields update`  -  Updates a product field. For more information, see the tutorial for updating custom fields' values .

**products**  -  Products are the goods or services you are dealing with. Each product can have N different price points - firstly, each product can have a price in N different currencies, and secondly, each product can have N variations of itself, each having N prices in different currencies. Note that only one price per variation per currency is supported. Products can be instantiated to deals. In the context of instatiation, a custom price, quantity, duration and discount can be applied.

- `pipedrive-cli products add`  -  Adds a new product. (Restored core v1 create endpoint.)
- `pipedrive-cli products delete`  -  Marks a product as deleted. (Restored core v1 delete endpoint.)
- `pipedrive-cli products get`  -  Returns the details of a specific product. (Restored core v1 detail endpoint.)
- `pipedrive-cli products get-all`  -  Returns all products. (Restored core v1 list endpoint.)
- `pipedrive-cli products update`  -  Updates the properties of a product. (Restored core v1 update endpoint.)

**project-templates**  -  Project templates allow you to have reusable and dynamic structure to simplify creation of a project. Project template can contain information about activities, tasks and groups that will be used when creating a project.

- `pipedrive-cli project-templates get`  -  Returns all not deleted project templates. This is a cursor-paginated endpoint.
- `pipedrive-cli project-templates get-projecttemplates`  -  Returns the details of a specific project template.

**projects**  -  Projects represent ongoing, completed or canceled projects attached to an organization, person or to deals. Each project has an owner and must be placed in a phase. Each project consists of standard data fields but can also contain a number of custom fields. The custom fields can be recognized by long hashes as keys.

- `pipedrive-cli projects add`  -  Adds a new project.
- `pipedrive-cli projects delete`  -  Marks a project as deleted.
- `pipedrive-cli projects get`  -  Returns all projects. This is a cursor-paginated endpoint.
- `pipedrive-cli projects get-board`  -  Returns the details of a specific project board.
- `pipedrive-cli projects get-boards`  -  Returns all projects boards that are not deleted.
- `pipedrive-cli projects get-id`  -  Returns the details of a specific project. Also note that custom fields appear as long hashes in the resulting data.
- `pipedrive-cli projects get-phase`  -  Returns the details of a specific project phase.
- `pipedrive-cli projects get-phases`  -  Returns all active project phases under a specific board.
- `pipedrive-cli projects update`  -  Updates a project.

**recents**  -  Recent changes across all item types in Pipedrive (deals, persons, etc).

- `pipedrive-cli recents`  -  Returns data about all recent changes occurred after the given timestamp.

**roles**  -  Roles are a part of the Visibility groups’ feature that allow the admin user to categorize other users and dictate what items they will be allowed access to see.

- `pipedrive-cli roles add`  -  Adds a new role.
- `pipedrive-cli roles delete`  -  Marks a role as deleted.
- `pipedrive-cli roles get`  -  Returns all the roles within the company.
- `pipedrive-cli roles get-id`  -  Returns the details of a specific role.
- `pipedrive-cli roles update`  -  Updates the parent role and/or the name of a specific role.

**stages**  -  Stage is a logical component of a pipeline, and essentially a bucket that can hold a number of deals. In the context of the pipeline a stage belongs to, it has an order number which defines the order of stages in that pipeline.

- `pipedrive-cli stages get`  -  Returns the details of a specific stage. (Restored core v1 detail endpoint.)
- `pipedrive-cli stages get-all`  -  Returns all stages. (Restored core v1 list endpoint.)

**tasks**  -  Tasks represent actions that need to be completed and must be associated with a project. Tasks have an optional due date, can be assigned to a user and can have subtasks.

- `pipedrive-cli tasks add`  -  Adds a new task.
- `pipedrive-cli tasks delete`  -  Marks a task as deleted. If the task has subtasks then those will also be deleted.
- `pipedrive-cli tasks get`  -  Returns all tasks. This is a cursor-paginated endpoint.
- `pipedrive-cli tasks get-id`  -  Returns the details of a specific task.
- `pipedrive-cli tasks update`  -  Updates a task.

**user-connections**  -  Manage user connections.

- `pipedrive-cli user-connections`  -  Returns data about all connections for the authorized user.

**user-settings**  -  View user settings.

- `pipedrive-cli user-settings`  -  Lists the settings of an authorized user. Example response contains a shortened list of settings.

**users**  -  Users are people with access to your Pipedrive account. A user may belong to one or many Pipedrive accounts, so deleting a user from one Pipedrive account will not remove the user from the data store if he/she is connected to multiple accounts. Users should not be confused with persons.

- `pipedrive-cli users add`  -  Adds a new user to the company, returns the ID upon success.
- `pipedrive-cli users find-by-name`  -  Finds users by their name.
- `pipedrive-cli users get`  -  Returns data about all users within the company.
- `pipedrive-cli users get-current`  -  Returns data about an authorized user within the company with bound company data: company ID, company name, and domain.
- `pipedrive-cli users get-id`  -  Returns data about a specific user within the company.
- `pipedrive-cli users update`  -  Updates the properties of a user. Currently, only `active_flag` can be updated.

**webhooks**  -  See <a href="https://pipedrive.readme.io/docs/guide-for-webhooks-v2?ref=api_reference" target="_blank" rel="noopener noreferrer">the guide for Webhooks</a> for more information.

- `pipedrive-cli webhooks add`  -  Creates a new Webhook and returns its details.
- `pipedrive-cli webhooks delete`  -  Deletes the specified Webhook.
- `pipedrive-cli webhooks get`  -  Returns data about all the Webhooks of a company.


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
pipedrive-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Morning triage

```bash
pipedrive-cli stale --quiet-days 14 --limit 10
```

The ten open deals most at risk because nobody has touched them recently.

### Agent-narrow a verbose forecast

```bash
pipedrive-cli forecast --period this-quarter --agent --select pipelines.pipeline_name,pipelines.weighted_value,expected_to_close
```

Pairs --agent with --select dotted paths so an agent gets only the weighted-value fields instead of the full nested forecast payload.

### Find the bottleneck stage

```bash
pipedrive-cli aging --agent
```

Surfaces deals dwelling past their stage's median time-in-stage, so you can see where the pipeline is clogging.

### Incremental change feed for a script

```bash
pipedrive-cli changes --since 24h --json | jq '.entities[] | select(.entity=="deals")'
```

Only the records that moved in the last day, ready to pipe into downstream automation.

### Dedupe contacts before an import

```bash
pipedrive-cli dupes --entity persons --json
```

Clusters of likely-duplicate people by normalized name/email/phone, machine-readable for a cleanup script.

## Auth Setup

Pipedrive uses a personal API token sent in the `x-api-token` header. Copy yours from Personal preferences -> API (https://app.pipedrive.com/settings/personal/api) and set it as `PIPEDRIVE_API_KEY`. Read commands and `sync` need a valid token; offline store queries (analytics, search, and the cached intelligence commands after a sync) work without re-hitting the API.

Run `pipedrive-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  pipedrive-cli activities get <id> --agent --select id,name,status
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
pipedrive-cli feedback "the --since flag is inclusive but docs say exclusive"
pipedrive-cli feedback --stdin < notes.txt
pipedrive-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/pipedrive-cli/feedback.jsonl`. They are never POSTed unless `PIPEDRIVE_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `PIPEDRIVE_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
pipedrive-cli profile save briefing --json
pipedrive-cli --profile briefing activities get <id>
pipedrive-cli profile list --json
pipedrive-cli profile show briefing
pipedrive-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `pipedrive-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/sales-and-crm/pipedrive/cmd/pipedrive-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add pipedrive-mcp -- pipedrive-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which pipedrive-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   pipedrive-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `pipedrive-cli <command> --help`.
