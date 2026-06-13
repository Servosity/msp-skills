# PandaDoc CLI

**Every PandaDoc endpoint, plus an offline document pipeline no other PandaDoc tool has  -  stalled deals, aging, recipient engagement, and open quote value from a local store.**

pandadoc-cli wraps the full PandaDoc Public API and syncs documents, templates, contacts, content library, and webhooks into a local SQLite store. On top of that it answers questions the API can't: which documents are stalled (stalled), how long they've aged (aging), which recipients never sign (engagement), and how much quote value is in flight (value).

Learn more at [PandaDoc](https://developers.pandadoc.com/).

Created by [@dstevens](https://github.com/dstevens) (Damien Stevens).
Contributors: [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

The recommended path installs both the `pandadoc-cli` binary and the `pp-pandadoc` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install pandadoc
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install pandadoc --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install pandadoc --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install pandadoc --agent claude-code
npx -y @mvanhorn/printing-press-library install pandadoc --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/sales-and-crm/pandadoc/cmd/pandadoc-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/pandadoc-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install pandadoc --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-pandadoc --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-pandadoc --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install pandadoc --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/pandadoc-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `PANDADOC_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/sales-and-crm/pandadoc/cmd/pandadoc-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "pandadoc": {
      "command": "pandadoc-mcp",
      "env": {
        "PANDADOC_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Authenticate with a PandaDoc API key. Set PANDADOC_API_KEY and every call sends the Authorization: API-Key <key> header for you.

## Quick Start

```bash
# confirm your API key is set and PandaDoc is reachable
pandadoc-cli doctor

# pull documents, templates, contacts, and webhooks into the local store
pandadoc-cli sync

# see the whole document funnel at a glance
pandadoc-cli pipeline

# surface documents sent but not completed in two weeks
pandadoc-cli stalled --days 14

# list completed documents (status 2 = document.completed) as JSON for piping
pandadoc-cli documents list --status 2 --json

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Pipeline intelligence
- **`pipeline`**  -  See your whole document funnel at a glance  -  how many are in draft, sent, viewed, completed, or declined.

  _Reach for this when you need a single status breakdown instead of paging the documents endpoint and counting yourself._

  ```bash
  pandadoc-cli pipeline --agent
  ```
- **`stalled`**  -  Find documents that were sent but never completed within N days  -  the deals quietly dying.

  _Use before a pipeline review to surface proposals that need a nudge._

  ```bash
  pandadoc-cli stalled --days 14 --agent
  ```
- **`aging`**  -  Show how long each document has sat in its current status, bucketed by age.

  _Pick this to spot bottlenecks  -  where documents pile up between sent and signed._

  ```bash
  pandadoc-cli aging --agent
  ```
- **`template-stats`**  -  Per-template document counts and completion rates  -  which templates actually close.

  _Reach for this to retire low-converting templates and double down on winners._

  ```bash
  pandadoc-cli template-stats --agent
  ```
- **`value`**  -  Sum the quote/pricing totals across all open (non-completed) documents  -  your in-flight dollar value.

  _Use for a fast forecast of pipeline value without exporting to a spreadsheet._

  ```bash
  pandadoc-cli value --status sent --agent
  ```
- **`since`**  -  Show what changed in the last N hours  -  new documents and status transitions.

  _Run at the start of the day to catch every document that moved overnight._

  ```bash
  pandadoc-cli since 4h --agent
  ```
- **`forecast`**  -  Bucket open quote dollars into healthy, aging, and stalled tiers by deal age.

  _Reach for this when a flat open-value total isn't enough and you need dollars-at-risk by deal age._

  ```bash
  pandadoc-cli forecast --agent
  ```

### Relationship intelligence
- **`engagement`**  -  Rank recipients by how often they open and sign vs. let documents sit unread.

  _Use to find which clients consistently stall so you can change how you route to them._

  ```bash
  pandadoc-cli engagement --agent
  ```
- **`cold-clients`**  -  Rank clients by how long since they last signed anything  -  spot the accounts going quiet.

  _Reach for this when you need account-level recency (who went cold), not per-document status or per-recipient rates._

  ```bash
  pandadoc-cli cold-clients --days 30 --agent
  ```
- **`followup`**  -  A ranked nudge worklist: stalled documents joined to recipient emails and days-since-sent, ready for outreach.

  _Reach for this when you need who-to-nudge with contact emails attached, not just a stalled-document list._

  ```bash
  pandadoc-cli followup --days 7 --agent
  ```

### Operational safety
- **`webhook-coverage`**  -  Compare your active webhook subscriptions against the full event catalog to find gaps.

  _Use to confirm you won't miss a document.state_changed event before relying on automations._

  ```bash
  pandadoc-cli webhook-coverage --agent
  ```
- **`reminder-gaps`**  -  Find sent-but-incomplete documents that have no active auto-reminder set  -  queries the live API per document (needs PANDADOC_API_KEY).

  _Reach for this to verify PandaDoc is auto-nudging signers before you nudge manually; unlike the other store-backed analytics it makes live API calls._

  ```bash
  pandadoc-cli reminder-gaps --max-scan-docs 25 --agent
  ```

## Recipes


### Morning pipeline check

```bash
pandadoc-cli sync && pandadoc-cli pipeline --agent
```

Refresh the store, then print the status funnel for a fast standup view.

### Find stalling proposals

```bash
pandadoc-cli stalled --days 10 --json --select id,name,status,date_modified
```

List documents sent but unsigned for 10 days, narrowed to the fields that matter.

### Forecast in-flight value

```bash
pandadoc-cli value --status sent --agent
```

Sum quote totals across sent documents for a quick pipeline-dollar forecast.

### Who never signs

```bash
pandadoc-cli engagement --json --select email,documents,completed
```

Rank recipients by completion rate to spot chronically slow signers.

### Confirm webhook coverage

```bash
pandadoc-cli webhook-coverage --agent
```

Check subscribed event types against the catalog before trusting an automation.

## Usage

Run `pandadoc-cli --help` for the full command reference and flag list.

## Commands

### contacts

Operations related to managing and retrieving contact details.

- **`pandadoc-cli contacts create`** - This method adds a contact into a contacts list.
- **`pandadoc-cli contacts delete`** - This method deletes a contact.
- **`pandadoc-cli contacts details`** - Returns contact details by its ID.
- **`pandadoc-cli contacts list`** - This method returns a list of contacts associated with a workspace.
- **`pandadoc-cli contacts update`** - This method updates a contact details.

### content-library-items

Operations for managing content library items, including retrieving details, checking status, and creating items via file upload.

- **`pandadoc-cli content-library-items create`** - This API endpoint allows users to create an empty item in the content library.
No actual content or data is required to be provided in the initial creation.
- **`pandadoc-cli content-library-items list`** - The endpoint retrieves items from the content library in PandaDoc. This endpoint supports filtering options to narrow down the results, allowing users to search by query, tags, folder, and more.

> ### ⚠️ Please avoid empty values for the parameters
> API returns "400" error when any of the parameters has an empty value. Please remove such a parameter from the request or add a value.
- **`pandadoc-cli content-library-items status`** - Requesting the CLI status helps verify that a CLI is in the expected state before invoking additional API methods.

## Available CLI Statuses

The following is a complete list of all possible CLI statuses returned:

| CLI Status | Status Description |
|-----------------|--------------------|
| `cli.UPLOADED`  | The CLI upload process has been initiated and is currently in progress. It will soon transition to the `cli.PROCESSED` state. |
| `cli.PROCESSED` | The CLI has been successfully uploaded and created. At this stage, all aspects of the CLI are editable. |
| `cli.ERROR`     | The CLI upload process has failed. Please refer to the error details in the response for more information. |

### content-library-items-upload

Manage content library items upload

- **`pandadoc-cli content-library-items-upload`** - This asynchronous endpoint allows users to create a new CLI by uploading a file.
The uploaded file is processed in the background to generate the CLI.
The maximum allowable file size for upload is 100 MB.
Field tags and form fields are not supported yet.
Once the file is uploaded, the processing will happen asynchronously, and users need to check the status of the CLI creation.

### documents

Operations for managing documents, including appending content library items and creating document sessions for embedded signing.

- **`pandadoc-cli documents bulk-delete`** - Delete multiple documents in one request by sending a JSON array. Each element must be an object with an `id` field (the document ID, same value as elsewhere in the Documents API).

The caller must have permission to delete each document. Documents must belong to the authenticated workspace.

This batch operation returns **200 OK** with a JSON body containing an `id` array of deleted document IDs. That differs from [Delete Document](#operation/deleteDocument) (`DELETE /public/v1/documents/{id}`), which returns **204 No Content** for a single document.
- **`pandadoc-cli documents create`** - ## Create from a template
> See the [Create document from template](https://developers.pandadoc.com/docs/create-document-from-template) tutorial for details on how to use this endpoint, as well as a sample template.

## Create from a URL
> See the [Create from public PDF](https://developers.pandadoc.com/docs/create-and-send-a-document-from-a-publicly-available-pdf) guide for info about roles and fields, as well as PDF examples.
- **`pandadoc-cli documents create-folder`** - Create a new folder to store your documents.

For the full list of folder operations and their limitations, see [Organize Documents and Folders](https://developers.pandadoc.com/docs/organize-folders).
- **`pandadoc-cli documents delete`** - Delete a document by ID.
- **`pandadoc-cli documents list`** - This endpoint will let you list and search for the documents.
### [Here](https://developers.pandadoc.com/docs/list-search-documents-api) you can find how to filter, search and order documents.
- **`pandadoc-cli documents list-by-linked-object`** - Get a list of documents connected to a linked object - an entity from an integration.
- **`pandadoc-cli documents list-folders`** - Get the list of folders which contain Documents in your account.

> 📘 
> 
> The root folder is not listed in the response.

For the full list of folder operations and their limitations, see [Organize Documents and Folders](https://developers.pandadoc.com/docs/organize-folders).
- **`pandadoc-cli documents rename-folder`** - Rename Documents Folder.

For the full list of folder operations and their limitations, see [Organize Documents and Folders](https://developers.pandadoc.com/docs/organize-folders).
- **`pandadoc-cli documents status`** - It is useful to request document status to ensure a document is in the expected state before calling additional API methods. 

### Required Document Statuses

Here are some common methods and the `document.status` required to proceed:

| API Method           | Required Document State |
| :------------------- | :---------------------- |
| Send A Document      | `document.draft`        |
| Get Document Details | `document.draft`        |
| Embed A Document     | `document.sent`         |
| Download A Document  | `document.completed`    |

> 📘 Polling vs Webhooks
> 
> If you are using the `GET` document status endpoint for [**polling**](https://en.wikipedia.org/wiki/Polling_(computer_science)), we also support and recommend using **webhooks** for event-driven needs: <https://developers.pandadoc.com/docs/listen-document-status-changes#/>

### Available Document Statuses

The following is a complete list of all possible document statuses returned:

| Document Status             | Status Description                                                                                                                                                                                                               |
| :-------------------------- | :------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `document.uploaded`         | The document has just been created or uploaded. It is in processing and will be in `document.draft` state soon.                                                                                                           |
| `document.error`            | The document creation has failed. This status is terminal, you should stop polling after getting it.
| `document.draft`            | The document is in a draft state. All aspects of the document can be edited in this state. Our API does not support edits after the document has been created, but it can still be edited manually on <https://app.pandadoc.com> |
| `document.sent`             | The document has been "sealed" and optionally sent. No further document edits can occur except for document recipient(s) filling out or signing the document.                                                                    |
| `document.viewed`           | Document recipient(s) have viewed the sent document.                                                                                                                                                                             |
| `document.waiting_approval` | The document has an [automatic approval workflow](https://support.pandadoc.com/en/articles/9714799-approval-workflow) and has not yet been approved.                                                      |
| `document.rejected`         | The document has an [automatic approval workflow](https://support.pandadoc.com/en/articles/9714799-approval-workflow) and was rejected.                                                                   |
| `document.approved`         | The document has an [automatic approval workflow](https://support.pandadoc.com/en/articles/9714799-approval-workflow) and was approved.                                                                   |
| `document.waiting_pay`      | The document has a [Stripe payment](https://support.pandadoc.com/en/articles/9714942-stripe-checkout-payments) option and is awaiting payment.                                                                              |
| `document.paid`             | The document has a [Stripe payment](https://support.pandadoc.com/en/articles/9714942-stripe-checkout-payments) option and was paid.                                                                                         |
| `document.completed`        | The document has been completed by all recipients.                                                                                                                                                                               |
| `document.voided`           | The document expired and is no longer available for completion or signature.                                                                                                                                                     |
| `document.declined`         | The document was [manually marked](https://support.pandadoc.com/en/articles/9714842-manually-change-document-status) as "Declined"                                                                                    |
| `document.external_review`  | The document is reviewed by it's recipient using Suggest Edit feature                                                                                                                                                            |
- **`pandadoc-cli documents transfer-all-ownership`** - This method transfers ownership of all documents from one member to another.
- **`pandadoc-cli documents update`** - Use the PATCH method to update a PandaDoc document.

> 🚧 Document status
> 
> You can only update a document in the Draft status (`document.draft`). 
> 
> After creating a new document, it usually retains a `document.uploaded` status for 3-5 seconds while the document syncs across PandaDoc servers. When the document is available for further API calls, the document moves to the `document.draft` state. Use [Document Status](https://developers.pandadoc.com/reference/document-status) or Webhooks to check document status.

### documents-upload

Manage documents upload

- **`pandadoc-cli documents-upload`** - ## Create from an upload
> See the [Create from PDF](https://developers.pandadoc.com/docs/create-document-from-file) tutorial for the usage specifics and sample PDF files.

**Note**: A file you upload is not stored in your PandaDoc account, so you have to upload it with every request.

### documents-upload-markdown

Manage documents upload markdown

- **`pandadoc-cli documents-upload-markdown`** - ## Create from a Markdown upload

Upload a Markdown (`.md`) file to create a new document. The file content will be converted into a PandaDoc document.

**Note**: A file you upload is not stored in your PandaDoc account, so you have to upload it with every request.

> **Alpha:** Markdown file upload is currently in alpha.
> This functionality may change or be removed without notice.

### forms

Operations for managing and retrieving forms, including filtering and sorting options.

- **`pandadoc-cli forms`** - Retrieve a paginated list of forms with optional filtering and sorting options.

### logs

Manage logs

- **`pandadoc-cli logs details`** - Returns details of the specific API log event.
- **`pandadoc-cli logs list`** - Get the list of all logs within the selected workspace.\ Optionally filter by date, page, and `#` of items per page.

### members

Operations for managing and retrieving details about workspace members.

- **`pandadoc-cli members details`** - A method to retrieve a member's details by ID.

**User** - is an account with a license in the Organization.  
**Member** - is a User with a predefined Role in the Workspace.

| Parameter | Description |
|---|---|
| `user_id` | A unique identifier of the `user` in the **organization** |
| `membership_id` | A unique identifier of the `user` in the **workspace** |
| `email` | A user email address |
| `first_name` | A user's first name |
| `last_name` | A user's last name |
| `is_active` | A boolean value that identifies if a member is active and not blocked |
| `workspace` | A unique identifier of the user's current active workspace |
| `workspace_name` | A name of the user's current active workspace |
| `email_verified` | A boolean value that identifies if the email is verified |
| `role` | A member's role in the workspace |
| `user_license` | A user license in the organization:  <br/>`Full (Standard)`;  <br/>`Read-only`;  <br/>`eSignature`;  <br/>`Guest`;  <br/>`Creator` |
| `date_created` | A date when a member was added to the workspace |
| `date_modified` | Last modified date of a member |
- **`pandadoc-cli members details-current`** - Returns the member details of the current user (the owner of the API key).

**User** - is an account with a license in the Organization.  
**Member** - is a User with a predefined Role in the Workspace.

| Parameter | Description |
|---|---|
| `user_id` | A unique identifier of the `user` in the **organization** |
| `membership_id` | A unique identifier of the `user` in the **workspace** |
| `email` | A user email address |
| `first_name` | A user's first name |
| `last_name` | A user's last name |
| `is_active` | A boolean value that identifies if a member is active and not blocked |
| `workspace` | A unique identifier of the user's current active workspace |
| `workspace_name` | A name of the user's current active workspace |
| `email_verified` | A boolean value that identifies if the email is verified |
| `role` | A member's role in the workspace |
| `user_license` | A user license in the organization:  <br/>`Full (Standard)`;  <br/>`Read-only`;  <br/>`eSignature`;  <br/>`Guest`;  <br/>`Creator` |
| `date_created` | A date when a member was added to the workspace |
| `date_modified` | Last modified date of a member |
- **`pandadoc-cli members list`** - Retrieve all members details of the workspace implied by the OAuth token or API key.\
For each member, the `workspace` parameter shows their active workspace, that is the workspace they are currently working in.\
This means the `workspace` value can differ from the workspace implied by your API key.

### public

Manage public

- **`pandadoc-cli public add-dsv-named-items`** - Adds one or more named items to the specified document by ID. 
These items define the document's structure and hierarchy (e.g., sections or headings) 
for display or navigation purposes within the Document Structure View (DSV).
- **`pandadoc-cli public create-catalog-item`** - Create a new catalog item.
- **`pandadoc-cli public create-export-docx-task`** - > ⏱️ Export as DOCX is a non-blocking (asynchronous) operation
> The document generation process may take some time.
> With a successful request, you receive a response with task ID, status **created** and document id. After process completes, usually in a few minutes, the task status moves to the **done** state.
> You can download documents up to 300 pages. For documents of 301+ pages, you will receive an error “400: The number of pages more then limit 300”
- **`pandadoc-cli public create-notarization-request`** - Create a notarization request to connect with a notary and complete online notarizations for your signers within minutes.

> 🚧 **Important:** This endpoint supports only documents in draft status.

## Prerequisites

> 🚧 Before you start
> 
> Ensure the following before creating a notarization request:
> 
> - Install the Notary On-Demand or Notary add-on
> - Create a document for notarization and get its `document_id`. To create a document, use the [Create Document from Template](https://developers.pandadoc.com/reference/create-document-from-pandadoc-template) or [Create Document from File Upload](https://developers.pandadoc.com/reference/create-document-from-pdf) endpoint.

## Request Details

For the notarization request, include in the request body:

- `document_id`
- At least one `invitees`, specifying their `email`, `first_name`, and `last_name`
- Optionally, include a `message` for your signers
- Optionally, using `disable_invitees_notifications` you can disable all notifications for invitees including email with invitation for notarization. This is useful when you are using alternative delivery methods.
- If in-house notary must be assigned to this request, include the `notary` object with the notary's `id`, `scheduled_at` timestamp, and an optional `message` for the notary

After the API call is executed, your signers will receive an email invitation for notarization. Alternatively, you can directly share the `notarization_link` with your signers, which is available in the 201 response body.

Upon successful notarization, you will receive an email with a link to the notarized document.

## Usage Tips

> 📘 Best Practices
> 
> - Ensure that signers are added as both invitees in the request body and recipients in the document to avoid inconveniences during notary sessions
> - Signers will receive an email with a notary link upon a successful API call; this link is also in the 201 response
> - In case if notary is not specified in the request, signers will use the link to connect with commissioned online notaries, available Mon-Fri, 9 AM - 9 PM Central Time, typically responding within 2 minutes
> - If notary is specified, signers will use the link to connect with your in-house notary at the scheduled time

## Limits

A maximum of 100 API calls per minute is permitted. Exceeding this limit triggers a 429 Too Many Requests error.

## Troubleshooting

**Solutions for 4xx Response Codes:**

- **403 Forbidden (Inactive Add-on)**: Ensure the Notary On-Demand or Notary add-on is installed
- **403 Forbidden (Transactions Limit)**: Purchase additional transactions either through the Notary UI or by contacting the Sales team
- **429 Too Many Requests**: If you hit the limit, hold your API calls, then send them after waiting for the retry time

> 📘 To learn more about PandaDoc Notary On-Demand, visit our [website](https://notary.pandadoc.com/notary-on-demand/).
- **`pandadoc-cli public delete-catalog-item`** - Delete catalog item.
- **`pandadoc-cli public delete-notarization-request`** - Use this method to delete a notarization request.
Once notarization request is deleted it cannot be restored.

> 🚧 Notarization Request status
> 
> You can only delete a notarization request in status 'SENT', 'WAITING_FOR_NOTARY' or 'INCOMPLETE'. 
> If the notarization request is in any other status, the request will return a 400 Bad Request error.

By default all invitees will receive email notification about deletion of the notarization request. 
If you want to disable this notification, you can use the `disable_invitees_notifications` parameter when creating request (see [Create Notarization Request](https://developers.pandadoc.com/reference/create-notarization-request)).
- **`pandadoc-cli public details-log-v2`** - Returns details of the specific API log event.
- **`pandadoc-cli public document-settings-get`** - Retrieves the settings for a specified document. Supported fields: language, qualified_electronic_signature, expires_in (in days).
- **`pandadoc-cli public document-settings-update`** - Updates the settings for a specified document. Supported fields: language, qualified_electronic_signature, expires_in (in days).
- **`pandadoc-cli public get-catalog-item`** - Catalog Item Details
- **`pandadoc-cli public get-document-ai-metadata`** - Returns the AI metadata fields populated for the document. Each result combines the field definition (`id`, `key`, `field_type`, `settings`) with the extracted value (`value`, `acceptance_status`).

Only fields that have an extracted value for the document are returned.

The endpoint signals AI extraction state via the HTTP status code:

- `202 extraction_pending`  -  extraction is in progress. Clients
  should retry after the number of seconds indicated in the
  `Retry-After` header.

- `204 No Content`  -  extraction has terminally failed for this
  document. No body is returned; retrying will not help  -  contact
  support.

- `409 not_started`  -  extraction has not been triggered for this
  document (typically because the document is not yet completed).
- **`pandadoc-cli public get-document-content`** - Returns the document content for the specified document. Use query parameter `format` to select the content format.
- **`pandadoc-cli public get-document-summary`** - Returns a summary for the specified document. Use query parameter `type` to select summary granularity.
- **`pandadoc-cli public get-docx-export-task`** - > 📘 This endpoint returns the current state of a DOCX export task for a document.
> The endpoint supports downloading only multiple files if the document contains several sections. Downloading as a single file in this case is not possible.
- **`pandadoc-cli public list-document-audit-trail`** - Retrieves the full audit trail for a specified document. The audit trail includes detailed user actions
such as sending, viewing, signing, and editing, along with metadata like timestamps, IP addresses, and user identity.
This endpoint is accessible to authorized workspace administrators only.
- **`pandadoc-cli public list-logs-v2`** - Get the list of all logs within the selected workspace.\ Optionally filter by date, page, and `#` of items per page.
- **`pandadoc-cli public list-notaries`** - Retrieve a list of notaries associated with your organization.

### API-specific
- **401 Unauthorized on every call**  -  Set PANDADOC_API_KEY to a valid key; the CLI sends it as 'Authorization: API-Key <key>'.
- **Transcendence commands return empty**  -  Run 'pandadoc-cli sync' first  -  pipeline/stalled/aging read the local store, not the live API.
- **Rate limited (429)**  -  PandaDoc throttles bursts; re-run sync, which backs off and resumes from the last cursor.

## Important Notes

- At the moment, notaries can be added to organization only manually through the PandaDoc Notary UI. 
- Organization must have Notary addon enabled to use this endpoint
- **`pandadoc-cli public list-notarization-requests`** - Retrieve a paginated list of notarization requests for your organization.

Results can be filtered by status, creator, or document, and sorted by status or by the request's creation or completion dates.

## Permissions

> 🔒 **Requirements**
>
> - The **Notary On-Demand** or **Notary** add-on must be enabled for your organization. Without an active add-on, the endpoint returns `403 Forbidden`.
> - The request is executed with the permissions of the API key owner. By default, the response only includes notarization requests created by the API key owner. To list notarization requests created by any user in the organization, the API key owner must have the **Can view any notarization request** permission.

## Limits

A maximum of 100 API calls per minute is permitted. Exceeding this limit triggers a 429 Too Many Requests error.
- **`pandadoc-cli public notarization-request-details`** - Get details about a notarization request by its `id`.

Details include:

- Basic notarization request information (status, creator, invitees).
- Signed documents information with links for downloading.
- Notarization session recording information with link for downloading.
- Timestamps associated with a notarization request.
- Termination reason and details when the notarization session was not completed successfully.

## Available Notarization Request Statuses

The following is a complete list of all possible notarization request statuses returned:

| Notarization Request Status | Status Description                                                                                                 |
| :-------------------------- | :----------------------------------------------------------------------------------------------------------------- |
| SENT                        | Notarization request has been created. Invitees are notified and can start the process of finding a notary.        |
| WAITING_FOR_NOTARY          | One of the invitees initialised the process of finding a notary.                                                   |
| ACCEPTED                    | Notarization request has been accepted by the notary. At this time nobody has joined the notarization session yet. |
| LIVE                        | Notarization session has started.                                                                                  |
| COMPLETED                   | Notarization session is finished. Documents have been successfully signed and ready for downloading.               |
| INCOMPLETE                  | Notarization session has started but was not completed successfully.                                               |

## Signed documents

Signed documents are the documents that were successfully signed during the notarization session. The signed document's info is available only if the notarization request has `COMPLETED` status, otherwise the returned list will be empty.  

In case you uploaded several documents for notarization then the `signed_documents` list will contain links for downloading for each document separately (with `SINGLE` document type) and link for the combined document (with `COMBINED` document type accordingly).

## Recording

Recording is the video of the notarization session. The recording info is available only if the notarization request has `COMPLETED` status and recording is available, otherwise the returned object will be empty.

> 📘 Links expire in 1 hour
> 
> **Note**: The signed document and recording links expire in 1 hour. After this time it will be not possible to download files using the returned urls. In this case you need to call endpoint again since each request generates a new link.

## Limits

A maximum of 100 API calls per minute is permitted. Exceeding this limit triggers a 429 Too Many Requests error.
- **`pandadoc-cli public search-catalog-items`** - This method searches for items in your [product catalog](https://support.pandadoc.com/en/articles/9714691-product-catalog).

Use the `query` parameter to search in title, SKU, description, category name, custom fields name and value. You can also search for items by their type, billing type, and category id.

Order search results, in both ascending and descending order, by these item properties:

- SKU
- Name
- Price
- Modification date

Use the `exclude_uuids` parameter to exclude particular uuids from the search request.
- **`pandadoc-cli public search-documents-ai`** - Find documents from a natural-language query. PandaDoc AI interprets
the query and returns the matching documents.

PandaDoc offers two document search tools: this AI-powered search
and the structured <a href="#/operations/listDocuments">List Documents</a>
endpoint.

<details><summary><strong>Document Search Tools Guide</strong>  -  click to expand</summary>

#### When to use AI Search (this endpoint)

- The query is in natural language (e.g., "show me all completed documents from Q2 2026")
- The query mentions people by name (e.g., "contracts shared with John Smith")
- The query uses relative date expressions (e.g., "last week", "this quarter")
- You want the system to automatically determine the best filtering strategy
- You want follow-up suggestions to help refine the search

#### When to use List Documents

- You already have exact, structured filter parameters (specific status codes, precise ISO-8601 date ranges)
- You need to paginate through a large result set (explicit page/count control)
- You need faster, more predictable response times
- You need deterministic, repeatable queries for automation workflows

#### Comparison

| Feature                  | AI Search                                           | List Documents                                   |
| ------------------------ | --------------------------------------------------- | ------------------------------------------------ |
| **Input**                | Single natural-language query                       | Search query + optional structured filters       |
| **Date handling**        | Understands relative dates ("last week", "Q2 2026") | Requires explicit ISO-8601 date range            |
| **Status filtering**     | Interprets from query ("completed", "sent")         | Requires numeric status codes                    |
| **Contact/owner search** | Resolves people by name                             | Not supported                                    |
| **Pagination**           | Returns top 100 results                             | Supports explicit page and page size (up to 100) |
| **Suggestions**          | Returns follow-up suggestions for refinement        | No suggestions                                   |
| **Best for**             | Conversational, exploratory search                  | Precise, structured, repeatable queries          |

</details>

The response may return fewer items than the total matches. The
search caps the returned list based on query complexity: `count`
shows the number of items returned in `results`. When the query
matches more documents than returned, use one of the returned
`suggestions` as a stricter follow-up query to narrow the results.

> 🚧 **Beta**
>
> This endpoint is currently in beta and may change without notice.
- **`pandadoc-cli public template-settings-get`** - Retrieves the settings for a specified template. Only the language field is currently supported.
- **`pandadoc-cli public template-settings-update`** - Updates the settings for a specified template. Only the language field is currently supported.
- **`pandadoc-cli public update-catalog-item`** - Update catalog item.

### sms-opt-outs

Manage sms opt outs

- **`pandadoc-cli sms-opt-outs`** - Retrieves a list of the most recent SMS opt-out changes for each phone numbers used in your workspace.

> 📘 You can filter results by time range using `timestamp_from` and `timestamp_to`.

### templates

Operations for managing templates, including listing, creating, and deleting templates.

- **`pandadoc-cli templates create`** - This operation allows you to create a new template by providing the necessary template details.
- **`pandadoc-cli templates create-folder`** - Create a new folder to store your templates.

For the full list of folder operations and their limitations, see [Organize Templates and Folders](https://developers.pandadoc.com/docs/organize-folders).
- **`pandadoc-cli templates delete`** - Delete a template
- **`pandadoc-cli templates list`** - Retrieves a list of templates. You can filter results by a search query, tags, or fields.
- **`pandadoc-cli templates list-folders`** - Get the list of folders that contain Templates in your account.

> 📘 
> 
> The root folder is not listed in the response.

For the full list of folder operations and their limitations, see [Organize Templates and Folders](https://developers.pandadoc.com/docs/organize-folders).
- **`pandadoc-cli templates rename-folder`** - Rename a templates folder.

For the full list of folder operations and their limitations, see [Organize Templates and Folders](https://developers.pandadoc.com/docs/organize-folders).
- **`pandadoc-cli templates status`** - ## Available Template Statuses

The following is a complete list of all possible template statuses returned:

| Template Status      | Status Description                                                                                                                      |
| :------------------- | :-------------------------------------------------------------------------------------------------------------------------------------- |
| `template.UPLOADED`  | The template upload process has been initiated and is currently in progress. It will soon transition to the `template.PROCESSED` state. |
| `template.PROCESSED` | The template has been successfully uploaded and created. At this stage, all aspects of the template are editable.                       |
| `template.ERROR`     | The template upload process has failed. Please refer to the error details in the response for more information.                         |
- **`pandadoc-cli templates update`** - Update a template. Currently supports updating template variables (`tokens`) and managing template roles.

> 🚧 Template status
> 
> You can only update a template in the PROCESSED status (`template.PROCESSED`). 
> 
> After creating a new template, it usually retains a `template.uploaded` status for 3-5 seconds while the template syncs across PandaDoc servers. When the template is available for further API calls, the template moves to the `template.PROCESSED` state. Use [Template Status](https://developers.pandadoc.com/reference/template-status) or Webhooks to check template status.

## Managing template roles

Pass a `roles` array to replace the full set of template roles in a single request:

  - Items with an existing `id` update that role (`name`, `signing_order`).
  - Items without an `id` create a new role.
  - Existing roles whose `id` is not in the array are deleted.

Role names must be unique within a template. Preassigned contacts and contact groups attached to roles in the template editor are preserved on updates and removed together with their role on deletion  -  they are not managed by this endpoint.

### templates-upload

Manage templates upload

- **`pandadoc-cli templates-upload`** - This asynchronous endpoint allows users to create a new template by uploading a file.

The uploaded file is processed in the background to generate the template.
The maximum allowable file size for upload is 100 MB.
Field tags and form fields are not supported yet.

Once the file is uploaded, the processing will happen asynchronously, and users need to check [the status of the template](https://developers.pandadoc.com/reference/template-status) creation.

### users

Manage users

- **`pandadoc-cli users create`** - Create users, and assign them roles, licenses, and workspaces.

- You must be an organization admin to create users.
- We check that the user email domain matches your organization domain.
- We check that the user email and phone number have a valid format.
- **`pandadoc-cli users details`** - Get detailed information about a specific user by their ID, including contact information, license type, and workspace roles.

You must be an organization admin to get user details.
- **`pandadoc-cli users list`** - Get a list of all users with membership in your organization, with their contact information, license type, and workspace roles.

You must be an organization admin to list users.

### webhook-events

Operations related to webhook events.

- **`pandadoc-cli webhook-events details`** - This operation fetches detailed information about a specific webhook event using its unique identifier.
- **`pandadoc-cli webhook-events list`** - This operation retrieves a paginated list of all webhook events.

### webhook-subscriptions

Operations for managing webhook subscriptions, including listing, retrieving details, and updating shared keys.

- **`pandadoc-cli webhook-subscriptions create`** - This operation creates a new webhook subscription by specifying its details.
- **`pandadoc-cli webhook-subscriptions delete`** - This operation deletes a specific webhook subscription identified by its UUID.
- **`pandadoc-cli webhook-subscriptions details`** - Get webhook subscription by uuid
- **`pandadoc-cli webhook-subscriptions list`** - This operation fetches a paginated list of webhook subscriptions.
- **`pandadoc-cli webhook-subscriptions update`** - This operation updates the details of a webhook subscription.

### workspaces

Manage workspaces

- **`pandadoc-cli workspaces create`** - Create a workspace in your organization.

- You need to be an Org Admin to create a workspace.
- You will be added to the new workspace with an Admin role.
- **`pandadoc-cli workspaces get-list`** - Get a list of all the active workspaces in the organization.


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
pandadoc-cli contacts list

# JSON for scripting and agents
pandadoc-cli contacts list --json

# Filter to specific fields
pandadoc-cli contacts list --json --select id,name,status

# Dry run  -  show the request without sending
pandadoc-cli contacts list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
pandadoc-cli contacts list --agent
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
pandadoc-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/pandadoc-public-pp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `PANDADOC_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `pandadoc-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `pandadoc-cli doctor` to check credentials
- Verify the environment variable is set: `echo $PANDADOC_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific

- **401 Unauthorized on every call**  -  Set PANDADOC_API_KEY to a valid key; the CLI sends it as 'Authorization: API-Key <key>'.
- **Transcendence commands return empty**  -  Run 'pandadoc-cli sync' first  -  pipeline/stalled/aging read the local store, not the live API.
- **Rate limited (429)**  -  PandaDoc throttles bursts; re-run sync, which backs off and resumes from the last cursor.

---

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**pandadoc-python-client**](https://github.com/PandaDoc/pandadoc-python-client)  -  Python
- [**pandadoc-node-client**](https://github.com/PandaDoc/pandadoc-node-client)  -  JavaScript

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
