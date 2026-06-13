---
name: pandadoc
description: "Every PandaDoc endpoint, plus an offline document pipeline no other PandaDoc tool has  -  stalled deals, aging, recipient engagement, and open quote value from a local store. Trigger phrases: `check my pandadoc pipeline`, `which proposals are stalled`, `list pandadoc documents`, `create a document from a template`, `how much quote value is open`, `use pandadoc`, `run pandadoc`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "PandaDoc"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - pandadoc-cli
    install:
      - kind: go
        bins: [pandadoc-cli]
        module: github.com/mvanhorn/printing-press-library/library/sales-and-crm/pandadoc/cmd/pandadoc-cli
---

# PandaDoc  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `pandadoc-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install pandadoc --cli-only
   ```
2. Verify: `pandadoc-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/sales-and-crm/pandadoc/cmd/pandadoc-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

pandadoc-cli wraps the full PandaDoc Public API and syncs documents, templates, contacts, content library, and webhooks into a local SQLite store. On top of that it answers questions the API can't: which documents are stalled (stalled), how long they've aged (aging), which recipients never sign (engagement), and how much quote value is in flight (value).

## When to Use This CLI

Reach for pandadoc-cli when an agent or operator needs to inspect or drive a PandaDoc document workflow from the terminal: creating documents from templates, sending and tracking signing status, or answering pipeline questions (stalled deals, aging, open quote value, recipient engagement) that the PandaDoc API exposes no single endpoint for. It is ideal for MSP/sales operations that live in proposals, quotes, MSAs, and SOWs.

## Anti-triggers

Do not use this CLI for:
- Drafting or reviewing contract LANGUAGE  -  this CLI moves documents through PandaDoc; it does not write or interpret legal text.
- E-signing a document yourself  -  signing happens in the recipient's PandaDoc session, not via this CLI.
- Documents stored outside PandaDoc (Google Docs, DocuSign, local PDFs not uploaded)  -  only the PandaDoc workspace is visible.
- Editing a document's body content or layout  -  the API exposes fields/tokens/pricing, not free-form content editing; use the PandaDoc editor.
- Real-time webhook delivery  -  webhook-coverage audits subscriptions; receiving events needs your own webhook endpoint, not this CLI.

## Unique Capabilities

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

## Command Reference

**contacts**  -  Operations related to managing and retrieving contact details.

- `pandadoc-cli contacts create`  -  This method adds a contact into a contacts list.
- `pandadoc-cli contacts delete`  -  This method deletes a contact.
- `pandadoc-cli contacts details`  -  Returns contact details by its ID.
- `pandadoc-cli contacts list`  -  This method returns a list of contacts associated with a workspace.
- `pandadoc-cli contacts update`  -  This method updates a contact details.

**content-library-items**  -  Operations for managing content library items, including retrieving details, checking status, and creating items via file upload.

- `pandadoc-cli content-library-items create`  -  This API endpoint allows users to create an empty item in the content library.
- `pandadoc-cli content-library-items list`  -  The endpoint retrieves items from the content library in PandaDoc.
- `pandadoc-cli content-library-items status`  -  Requesting the CLI status helps verify that a CLI is in the expected state before invoking additional API methods.

**content-library-items-upload**  -  Manage content library items upload

- `pandadoc-cli content-library-items-upload`  -  This asynchronous endpoint allows users to create a new CLI by uploading a file.

**documents**  -  Operations for managing documents, including appending content library items and creating document sessions for embedded signing.

- `pandadoc-cli documents bulk-delete`  -  Delete multiple documents in one request by sending a JSON array.
- `pandadoc-cli documents create`  -  > See the [Create document from template](https://developers.pandadoc.
- `pandadoc-cli documents create-folder`  -  Create a new folder to store your documents.
- `pandadoc-cli documents delete`  -  Delete a document by ID.
- `pandadoc-cli documents list`  -  This endpoint will let you list and search for the documents. ### [Here](https://developers.pandadoc.
- `pandadoc-cli documents list-by-linked-object`  -  Get a list of documents connected to a linked object - an entity from an integration.
- `pandadoc-cli documents list-folders`  -  Get the list of folders which contain Documents in your account. > 📘 > > The root folder is not listed in the response.
- `pandadoc-cli documents rename-folder`  -  Rename Documents Folder.
- `pandadoc-cli documents status`  -  It is useful to request document status to ensure a document is in the expected state before calling additional API
- `pandadoc-cli documents transfer-all-ownership`  -  This method transfers ownership of all documents from one member to another.
- `pandadoc-cli documents update`  -  Use the PATCH method to update a PandaDoc document.

**documents-upload**  -  Manage documents upload

- `pandadoc-cli documents-upload`  -  > See the [Create from PDF](https://developers.pandadoc.

**documents-upload-markdown**  -  Manage documents upload markdown

- `pandadoc-cli documents-upload-markdown`  -  Upload a Markdown (`.md`) file to create a new document. The file content will be converted into a PandaDoc document.

**forms**  -  Operations for managing and retrieving forms, including filtering and sorting options.

- `pandadoc-cli forms`  -  Retrieve a paginated list of forms with optional filtering and sorting options.

**logs**  -  Manage logs

- `pandadoc-cli logs details`  -  Returns details of the specific API log event.
- `pandadoc-cli logs list`  -  Get the list of all logs within the selected workspace. Optionally filter by date, page, and `#` of items per page.

**members**  -  Operations for managing and retrieving details about workspace members.

- `pandadoc-cli members details`  -  A method to retrieve a member's details by ID. **User** - is an account with a license in the Organization.
- `pandadoc-cli members details-current`  -  Returns the member details of the current user (the owner of the API key).
- `pandadoc-cli members list`  -  Retrieve all members details of the workspace implied by the OAuth token or API key.

**public**  -  Manage public

- `pandadoc-cli public add-dsv-named-items`  -  Adds one or more named items to the specified document by ID.
- `pandadoc-cli public create-catalog-item`  -  Create a new catalog item.
- `pandadoc-cli public create-export-docx-task`  -  > ⏱️ Export as DOCX is a non-blocking (asynchronous) operation > The document generation process may take some time.
- `pandadoc-cli public create-notarization-request`  -  Create a notarization request to connect with a notary and complete online notarizations for your signers within
- `pandadoc-cli public delete-catalog-item`  -  Delete catalog item.
- `pandadoc-cli public delete-notarization-request`  -  Use this method to delete a notarization request. Once notarization request is deleted it cannot be restored.
- `pandadoc-cli public details-log-v2`  -  Returns details of the specific API log event.
- `pandadoc-cli public document-settings-get`  -  Retrieves the settings for a specified document.
- `pandadoc-cli public document-settings-update`  -  Updates the settings for a specified document.
- `pandadoc-cli public get-catalog-item`  -  Catalog Item Details
- `pandadoc-cli public get-document-ai-metadata`  -  Returns the AI metadata fields populated for the document.
- `pandadoc-cli public get-document-content`  -  Returns the document content for the specified document. Use query parameter `format` to select the content format.
- `pandadoc-cli public get-document-summary`  -  Returns a summary for the specified document. Use query parameter `type` to select summary granularity.
- `pandadoc-cli public get-docx-export-task`  -  > 📘 This endpoint returns the current state of a DOCX export task for a document.
- `pandadoc-cli public list-document-audit-trail`  -  Retrieves the full audit trail for a specified document.
- `pandadoc-cli public list-logs-v2`  -  Get the list of all logs within the selected workspace. Optionally filter by date, page, and `#` of items per page.
- `pandadoc-cli public list-notaries`  -  Retrieve a list of notaries associated with your organization.
- `pandadoc-cli public list-notarization-requests`  -  Retrieve a paginated list of notarization requests for your organization.
- `pandadoc-cli public notarization-request-details`  -  Get details about a notarization request by its `id`.
- `pandadoc-cli public search-catalog-items`  -  This method searches for items in your [product catalog](https://support.pandadoc.
- `pandadoc-cli public search-documents-ai`  -  Find documents from a natural-language query. PandaDoc AI interprets the query and returns the matching documents.
- `pandadoc-cli public template-settings-get`  -  Retrieves the settings for a specified template. Only the language field is currently supported.
- `pandadoc-cli public template-settings-update`  -  Updates the settings for a specified template. Only the language field is currently supported.
- `pandadoc-cli public update-catalog-item`  -  Update catalog item.

**sms-opt-outs**  -  Manage sms opt outs

- `pandadoc-cli sms-opt-outs`  -  Retrieves a list of the most recent SMS opt-out changes for each phone numbers used in your workspace.

**templates**  -  Operations for managing templates, including listing, creating, and deleting templates.

- `pandadoc-cli templates create`  -  This operation allows you to create a new template by providing the necessary template details.
- `pandadoc-cli templates create-folder`  -  Create a new folder to store your templates.
- `pandadoc-cli templates delete`  -  Delete a template
- `pandadoc-cli templates list`  -  Retrieves a list of templates. You can filter results by a search query, tags, or fields.
- `pandadoc-cli templates list-folders`  -  Get the list of folders that contain Templates in your account. > 📘 > > The root folder is not listed in the response.
- `pandadoc-cli templates rename-folder`  -  Rename a templates folder.
- `pandadoc-cli templates status`  -  The following is a complete list of all possible template statuses returned:
- `pandadoc-cli templates update`  -  Update a template. Currently supports updating template variables (`tokens`) and managing template roles.

**templates-upload**  -  Manage templates upload

- `pandadoc-cli templates-upload`  -  This asynchronous endpoint allows users to create a new template by uploading a file.

**users**  -  Manage users

- `pandadoc-cli users create`  -  Create users, and assign them roles, licenses, and workspaces. - You must be an organization admin to create users.
- `pandadoc-cli users details`  -  Get detailed information about a specific user by their ID, including contact information, license type
- `pandadoc-cli users list`  -  Get a list of all users with membership in your organization, with their contact information, license type

**webhook-events**  -  Operations related to webhook events.

- `pandadoc-cli webhook-events details`  -  This operation fetches detailed information about a specific webhook event using its unique identifier.
- `pandadoc-cli webhook-events list`  -  This operation retrieves a paginated list of all webhook events.

**webhook-subscriptions**  -  Operations for managing webhook subscriptions, including listing, retrieving details, and updating shared keys.

- `pandadoc-cli webhook-subscriptions create`  -  This operation creates a new webhook subscription by specifying its details.
- `pandadoc-cli webhook-subscriptions delete`  -  This operation deletes a specific webhook subscription identified by its UUID.
- `pandadoc-cli webhook-subscriptions details`  -  Get webhook subscription by uuid
- `pandadoc-cli webhook-subscriptions list`  -  This operation fetches a paginated list of webhook subscriptions.
- `pandadoc-cli webhook-subscriptions update`  -  This operation updates the details of a webhook subscription.

**workspaces**  -  Manage workspaces

- `pandadoc-cli workspaces create`  -  Create a workspace in your organization. - You need to be an Org Admin to create a workspace.
- `pandadoc-cli workspaces get-list`  -  Get a list of all the active workspaces in the organization.


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
pandadoc-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

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

## Auth Setup

Authenticate with a PandaDoc API key. Set PANDADOC_API_KEY and every call sends the Authorization: API-Key <key> header for you.

Run `pandadoc-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  pandadoc-cli contacts list --agent --select id,name,status
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
pandadoc-cli feedback "the --since flag is inclusive but docs say exclusive"
pandadoc-cli feedback --stdin < notes.txt
pandadoc-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/pandadoc-cli/feedback.jsonl`. They are never POSTed unless `PANDADOC_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `PANDADOC_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
pandadoc-cli profile save briefing --json
pandadoc-cli --profile briefing contacts list
pandadoc-cli profile list --json
pandadoc-cli profile show briefing
pandadoc-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `pandadoc-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/sales-and-crm/pandadoc/cmd/pandadoc-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add pandadoc-mcp -- pandadoc-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which pandadoc-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   pandadoc-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `pandadoc-cli <command> --help`.
