---
name: jira-service-management
description: "Use when the user asks to triage Jira Service Management (JSM) queues by SLA risk, add an internal note to a service desk ticket, read a ticket's full history, manage JSM customers and client organizations, or look up and reconcile Assets (CMDB) objects: ranks open tickets across every queue by SLA time remaining, posts agent-only notes that error if JSM stores them as public, joins Assets objects to the tickets that touch them, and diffs Assets against an RMM or PSA export. Trigger phrases: `which tickets are about to breach SLA`, `add an internal note to this service desk ticket`, `show the full timeline for SD-123`, `which tickets touch this server in Assets`, `reconcile Assets against my RMM export`, `use jira service management`, `run jira-service-management-cli`, `jsm queues`, `jira service desk sla`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Jira Service Management"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - jira-service-management-cli
    install:
      - kind: script
        bins: [jira-service-management-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jira-service-management/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jira-service-management/install.ps1
---

# Jira Service Management - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `jira-service-management-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jira-service-management/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jira-service-management/install.ps1 | iex
   ```
3. Verify: `jira-service-management-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `jira-service-management-cli` and `jira-service-management-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Work queues, SLAs, internal notes, customers, organizations and Assets (CMDB) objects from the terminal or any MCP agent. 'breaches' ranks open tickets across every queue by SLA time remaining, 'request note' always posts agent-only notes (public=false) and errors if JSM stores one as public, and 'cmdb tickets' joins Assets objects to the tickets that touch them. Pagination walks past JSM's known paging bugs, and a local store powers offline search, 'sla-report' and per-client views.

## When to Use This CLI

Use this CLI when an agent needs to work Jira Service Management Cloud: triage queues by SLA risk ('breaches', 'servicedesk queue list-issues'), read a ticket ('request timeline'), add an internal note ('request note') or a customer reply ('request comment add --public'), move it ('request transition list' then 'request transition perform'), manage customers and client organizations, or look up Assets (CMDB) objects and their tickets. Assets command groups (aql, iql, usage, global, icon, config, api-import, progress, importsource, object, objectschema, objecttype, objecttypeattribute, cmdb tickets, cmdb reconcile) need a JSM Premium or Enterprise site plus JIRA_SERVICE_MANAGEMENT_CLOUD_ID and JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID ('cmdb discover --save'). 'sla-report' and 'client' read the local store, so run the named 'sync --resources ...' first; add 'comment' to --resources to index comments and internal notes for offline 'search' (one call per request).

## Anti-triggers

Do not use this CLI for:
- Jira Software work such as sprints, boards, epics or JQL issue search across non-service-desk projects; use a Jira platform CLI instead.
- JSM Operations (Opsgenie) alerts, on-call schedules and escalations; those live in a separate Operations API this CLI does not cover.
- Jira Service Management Data Center or Server instances; this CLI targets Atlassian Cloud only.
- Confluence knowledge base authoring; this CLI can search and view KB articles but not edit them.

## Unique Capabilities

### SLA and queue triage
- **`breaches`**  -  Rank open tickets that have a running SLA across your queues by time remaining, breached first, each ticket counted once (paused-only SLAs skipped unless --include-paused).

  _Reach for this when asked what is about to breach right now; one call replaces a queue walk plus one SLA call per ticket._

  ```bash
  jira-service-management-cli breaches --desk 3 --within 2h --agent
  ```
- **`sla-report`**  -  Breach rate, breached count, and median elapsed time vs median goal per desk, request type or SLA metric over a period. Reads the local store: run 'sync --resources request,servicedesk,requesttype' first; results are as of the last sync.

  _Use this for weekly SLA reporting; it answers in one local query what would otherwise be one API call per ticket._

  ```bash
  jira-service-management-cli sla-report --since 30d --group-by requesttype --agent
  ```

### MSP client hygiene
- **`roster`**  -  Get the complete customer list for a desk and spot customers with no organization, customers in two organizations, empty organizations, and unattached organizations that hold this desk's customers.

  _Use this for MSP customer onboarding/offboarding hygiene; the customers endpoint returns at most 50 people a page, and roster reads every page and de-duplicates._

  ```bash
  jira-service-management-cli roster 3 --agent
  ```
- **`client`**  -  One client organization's open tickets by status and request type, oldest open age, and how many are breaching. Reads the local store: run 'sync --resources organization,request,requesttype' first; counts are as of the last sync.

  _Use this before a client call to see how that client's tickets are doing._

  ```bash
  jira-service-management-cli client "Acme Corp" --agent
  ```

### Working a ticket
- **`request timeline`**  -  Read one ticket's whole story in order: comments labeled public or internal, status changes, approvals, participants and SLA state.

  _Use this before replying on a ticket so internal notes are never missed and never mistaken for customer-visible replies._

  ```bash
  jira-service-management-cli request timeline SD-123 --agent
  ```
- **`request note`**  -  Add an agent-only internal note to a request: it always posts public=false and errors if JSM stores the comment as public.

  _Pick this over a generic comment call whenever the note is for other agents, not the customer._

  ```bash
  jira-service-management-cli request note SD-123 "Escalated to network team" --dry-run
  ```

### Assets (CMDB)
- **`cmdb tickets`**  -  Go from a hostname, Assets key or AQL query to the matching Assets objects and every service desk ticket connected to them, with each ticket's SLA breach state. Needs Assets (JSM Premium or Enterprise) plus JIRA_SERVICE_MANAGEMENT_CLOUD_ID and JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID; run 'cmdb discover --save' once per client site.

  _Use this when a ticket names a device and you need its history across tickets._

  ```bash
  jira-service-management-cli cmdb tickets srv-01 --agent
  ```
- **`cmdb reconcile`**  -  Diff an Assets object type against an RMM or PSA CSV export: missing from Assets, missing from inventory, and attribute mismatches. Needs Assets (JSM Premium or Enterprise) plus JIRA_SERVICE_MANAGEMENT_CLOUD_ID and JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID; run 'cmdb discover --save' once per client site.

  _Use this for the weekly CMDB-vs-inventory check instead of a spreadsheet VLOOKUP._

  ```bash
  jira-service-management-cli cmdb reconcile --type Laptop --file rmm.csv --key Name --compare Serial,OS --agent
  ```
- **`cmdb discover`**  -  Find your site's cloudId and Assets workspaceId (Assets requires JSM Premium or Enterprise) and optionally save them with --save so Assets commands work.

  _Run this once when Assets commands report a missing cloudId or workspaceId._

  ```bash
  jira-service-management-cli cmdb discover --agent
  ```

## Command Reference

**api-import**  -  Manage api import

- `jira-service-management-cli api-import <id>`  -  Start configured imports. To see an ongoing import see the Progress resource

**aql**  -  Manage aql

- `jira-service-management-cli aql`  -  Deprecated by Atlassian (removal date passed); use 'object find-by-aql'.

**assets**  -  Manage assets

- `jira-service-management-cli assets`  -  Returns a list of Assets workspace IDs.

**config**  -  Manage config

- `jira-service-management-cli config create-referencetype`  -  Create a reference type (Assets).
- `jira-service-management-cli config create-statustype`  -  Create a new status
- `jira-service-management-cli config delete-statustype`  -  Delete an existing status
- `jira-service-management-cli config get-statustype`  -  Find a status by id
- `jira-service-management-cli config list-referencetypes`  -  Get reference type
- `jira-service-management-cli config list-statustypes`  -  Find all status
- `jira-service-management-cli config update-statustype`  -  Update an existing status

**customer**  -  Manage customer

- `jira-service-management-cli customer create`  -  This method adds a customer to the Jira Service Management instance by passing a JSON file including an email address
- `jira-service-management-cli customer create-skippermissioncheck`  -  Atlassian-internal endpoint (ASAP auth only); not callable with an API token.
- `jira-service-management-cli customer revoke-portal-only-access-for-user`  -  This method revokes portal-only access for a particular user

**global**  -  Manage global

- `jira-service-management-cli global <id>`  -  Update general configuration for object schema

**icon**  -  Manage icon

- `jira-service-management-cli icon get`  -  Load a single icon by id
- `jira-service-management-cli icon list-global`  -  Return all global icons i.e. icons not associated with a particular object schema

**importsource**  -  Manage importsource

- `jira-service-management-cli importsource <id>`  -  Retrieves a specific import source configuration by its ID.

**insight**  -  Manage insight

- `jira-service-management-cli insight`  -  This endpoint is deprecated, please use /assets/workspace/.

**instance**  -  Manage instance

- `jira-service-management-cli instance`  -  This method retrieves information about the Jira Service Management instance such as software version, builds

**iql**  -  Manage iql

- `jira-service-management-cli iql`  -  Deprecated by Atlassian (removal date passed); use 'object find-by-aql'.

**knowledgebase**  -  Manage knowledgebase

- `jira-service-management-cli knowledgebase list-articles`  -  Returns articles which match the given query string across all service desks.
- `jira-service-management-cli knowledgebase view-article`  -  View knowledge base article

**object**  -  Manage object

- `jira-service-management-cli object count-by-aql`  -  This API provides the total count of objects that match a specified AQL query.
- `jira-service-management-cli object create`  -  Create a new object in Assets
- `jira-service-management-cli object delete`  -  Delete the referenced object
- `jira-service-management-cli object find-by-aql`  -  Fetch Objects by AQL
- `jira-service-management-cli object get`  -  Load one object
- `jira-service-management-cli object list-connected-tickets`  -  Relation between Jira issues and Assets objects
- `jira-service-management-cli object navlist-aql`  -  Retrieve a list of objects based on an AQL. Deprecated from 30 September 2024. Please use POST /object/aql instead.
- `jira-service-management-cli object navlist-iql`  -  Deprecated by Atlassian (removal date passed); use 'object navlist-aql'. Deprecated. Use `/object/navlist/aql` instead.
- `jira-service-management-cli object update`  -  Update an existing object in Assets

**objectschema**  -  Manage objectschema

- `jira-service-management-cli objectschema create`  -  Create a new object schema
- `jira-service-management-cli objectschema delete`  -  Delete a schema
- `jira-service-management-cli objectschema get`  -  Find a schema by id
- `jira-service-management-cli objectschema list`  -  Resource to find object schemas in Assets
- `jira-service-management-cli objectschema update`  -  Update an object schema

**objecttype**  -  Manage objecttype

- `jira-service-management-cli objecttype create`  -  Create a new object type
- `jira-service-management-cli objecttype delete`  -  Delete an object type
- `jira-service-management-cli objecttype get`  -  Find an object type by id
- `jira-service-management-cli objecttype update`  -  Update an existing object type

**objecttypeattribute**  -  Manage objecttypeattribute

- `jira-service-management-cli objecttypeattribute create`  -  Create a new attribute on the given object type
- `jira-service-management-cli objecttypeattribute delete`  -  Delete an existing object type attribute
- `jira-service-management-cli objecttypeattribute update`  -  Update an existing object type attribute

**organization**  -  Manage organization

- `jira-service-management-cli organization create`  -  This method creates an organization by passing the name of the organization.
- `jira-service-management-cli organization delete`  -  This method deletes an organization. Note that the organization is deleted regardless of other associations it may have.
- `jira-service-management-cli organization get`  -  This method returns details of an organization.
- `jira-service-management-cli organization list`  -  This method returns a list of organizations in the Jira Service Management instance.

**progress**  -  Manage progress

- `jira-service-management-cli progress <id>`  -  Show ongoing import process

**request**  -  Manage request

- `jira-service-management-cli request create`  -  This method creates a customer request in a service desk.
- `jira-service-management-cli request get`  -  This method returns a customer request. **Permissions required**: Permission to access the specified service desk.
- `jira-service-management-cli request list`  -  This method returns all customer requests for the user executing the query.
- `jira-service-management-cli request validate`  -  Validates a customer request payload without creating (persisting) a request.

**requesttype**  -  Manage requesttype

- `jira-service-management-cli requesttype`  -  This method returns all customer request types used in the Jira Service Management instance

**servicedesk**  -  Manage servicedesk

- `jira-service-management-cli servicedesk get`  -  This method returns a service desk.
- `jira-service-management-cli servicedesk list`  -  This method returns all the service desks in the Jira Service Management instance that the user has permission to

**usage**  -  Manage usage

- `jira-service-management-cli usage`  -  Assets object counts for the tenant, total and per schema.


**breaches**


- `jira-service-management-cli breaches`  -  Rank open tickets that have a running SLA across your queues by time remaining, breached first, each ticket counted once (paused-only SLAs skipped unless --include-paused).


**client**


- `jira-service-management-cli client`  -  One client organization's open tickets by status and request type, oldest open age, and how many are breaching. Reads the local store: run 'sync --resources organization,request,requesttype' first; counts are as of the last sync.


**cmdb**


- `jira-service-management-cli cmdb tickets`  -  Go from a hostname, Assets key or AQL query to the matching Assets objects and every service desk ticket connected to them, with each ticket's SLA breach state. Needs Assets (JSM Premium or Enterprise) plus JIRA_SERVICE_MANAGEMENT_CLOUD_ID and JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID; run 'cmdb discover --save' once per client site.

- `jira-service-management-cli cmdb reconcile`  -  Diff an Assets object type against an RMM or PSA CSV export: missing from Assets, missing from inventory, and attribute mismatches. Needs Assets (JSM Premium or Enterprise) plus JIRA_SERVICE_MANAGEMENT_CLOUD_ID and JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID; run 'cmdb discover --save' once per client site.

- `jira-service-management-cli cmdb discover`  -  Find your site's cloudId and Assets workspaceId (Assets requires JSM Premium or Enterprise) and optionally save them with --save so Assets commands work.


**request**


- `jira-service-management-cli request timeline`  -  Read one ticket's whole story in order: comments labeled public or internal, status changes, approvals, participants and SLA state.

- `jira-service-management-cli request note`  -  Add an agent-only internal note to a request: it always posts public=false and errors if JSM stores the comment as public.


**roster**


- `jira-service-management-cli roster`  -  Get the complete customer list for a desk and spot customers with no organization, customers in two organizations, empty organizations, and unattached organizations that hold this desk's customers.


**sla-report**


- `jira-service-management-cli sla-report`  -  Breach rate, breached count, and median elapsed time vs median goal per desk, request type or SLA metric over a period. Reads the local store: run 'sync --resources request,servicedesk,requesttype' first; results are as of the last sync.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
jira-service-management-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### What breaches next

```bash
jira-service-management-cli breaches --desk 3 --within 4h --agent
```

Walks every queue on desk 3 once, de-duplicates tickets, keeps only tickets already breached or breaching within 4 hours, and ranks them by the smallest SLA time remaining.

### Read a ticket with its SLA, narrowed for an agent

```bash
jira-service-management-cli request get SD-123 --expand sla --agent --select issueKey,currentStatus.status,sla.values.name,sla.values.ongoingCycle.remainingTime.friendly
```

Fetches one request with SLA expanded and keeps only the status and each SLA's remaining time.

### Work one queue, narrowed for an agent

```bash
jira-service-management-cli servicedesk queue list-issues 3 10 --agent --select key,fields.summary
```

Lists the tickets in queue 10 of desk 3 with only their keys and summaries; use 'request timeline <key>' next, and 'request note <key> "text"' for an agent-only note.

### Reply to the customer, then move the ticket

```bash
jira-service-management-cli request comment add SD-123 --body "We replaced the toner; please confirm it prints." --public --dry-run
```

Previews a customer-visible reply; then run 'request transition list SD-123' to find the transition id and 'request transition perform SD-123 --id <id>' to move it.

### Weekly SLA rollup

```bash
jira-service-management-cli sla-report --since 7d --group-by requesttype --agent
```

Breach rate per request type over the last week from the local store; run 'sync --resources request,servicedesk,requesttype' first so the numbers are current.

## Auth Setup

Jira Service Management Cloud uses HTTP Basic auth with your Atlassian account email and an API token (create one at id.atlassian.com/manage-profile/security/api-tokens). Set JIRA_SERVICE_MANAGEMENT_SITE to your site subdomain (acme for acme.atlassian.net), JIRA_SERVICE_MANAGEMENT_EMAIL and JIRA_SERVICE_MANAGEMENT_API_TOKEN. Assets (CMDB) commands call api.atlassian.com, need a JSM Premium or Enterprise site, and also need JIRA_SERVICE_MANAGEMENT_CLOUD_ID and JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID: run 'cmdb discover' to print them or 'cmdb discover --save' to store them in the config file for that site (saved ids never apply to another site). The same credential works for both APIs. What you can see follows your Jira permissions: agents see their desks' requests and internal notes, customers see only their own requests. Set the site rather than JIRA_SERVICE_MANAGEMENT_BASE_URL: a base-URL override also redirects Assets calls.

Run `jira-service-management-cli doctor` to verify setup.

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
  jira-service-management-cli servicedesk list --agent --select id,projectKey,projectName
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit confirmation**  -  `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear. `--yes` gates only local confirmations such as `profile delete`; API writes and deletes send immediately, so preview them with `--dry-run` first
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

- Use `--home <dir>` for one invocation, or set `JIRA_SERVICE_MANAGEMENT_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `JIRA_SERVICE_MANAGEMENT_CONFIG_DIR`, `JIRA_SERVICE_MANAGEMENT_DATA_DIR`, `JIRA_SERVICE_MANAGEMENT_STATE_DIR`, `JIRA_SERVICE_MANAGEMENT_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `JIRA_SERVICE_MANAGEMENT_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains runtime state such as `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `jira-service-management-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "jira-service-management": {
        "command": "jira-service-management-mcp",
        "env": {
          "JIRA_SERVICE_MANAGEMENT_HOME": "/srv/jira-service-management"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `JIRA_SERVICE_MANAGEMENT_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `JIRA_SERVICE_MANAGEMENT_HOME`, or `doctor` will not find credentials left under the former root.

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
jira-service-management-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "jira-service-management-cli learnings confirm 12"] }
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
       materially more, record the divergence via `jira-service-management-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `jira-service-management-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "ttfr" teach satisfying a "Time to first response" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., one alias seeded under two entity kinds). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `jira-service-management-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
jira-service-management-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "ttfr") satisfies future queries under another alias (e.g., "first response", "Time to first response") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
jira-service-management-cli teach \
  --query "$QUERY" \
  --resource-type <type> \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
jira-service-management-cli teach-playbook \
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
jira-service-management-cli playbook amend \
  --query "$QUERY" \
  --add-note "$NOTE"
# (append shell `&` to background it)
```

What counts as worth amending: a behavior you OBSERVED this session that future-you would benefit from knowing. Examples worth amending:

- A workaround for a CLI surface that silently drops or misorders a flag.
- An undocumented endpoint shape (response wrapped in `{meta, results}`, payload nested two levels deeper than the docs claim).
- Observed schema drift (a field renamed, an index that shifted between releases, a category label that the API now returns lower-cased).

What does NOT belong in notes:

- The year-specific or entity-specific answer to the user's question. That's the response, not a learning.
- Per-ticket / per-customer / per-row data the playbook already retrieves at runtime.
- Statements that paraphrase what the existing notes already say.

The amend command appends to the family's existing notes with a timestamped marker (`[amend YYYY-MM-DDTHH:MMZ]: <text>`). Multiple amends accumulate; the audit trail is visible. If no playbook exists yet for the family, amend creates a notes-only one (so cold-start corrections still land).

#### PII discipline for amend notes

`playbook amend` notes are designed to potentially flow upstream as shared knowledge in future versions of the Printing Press. Keep them clean of user-identifying content so the upstream-contribution path stays open without retroactive scrubbing:

- **Do NOT embed** paths to user filesystems, personal API keys or tokens, user email addresses, user GitHub handles, or specific query histories tied to a single user.
- **Acceptable**: endpoint shapes, undocumented field names, API gotchas, observed schema drift, workarounds for CLI surfaces, generalizable pagination or retry tactics.

If a correction is only meaningful with user-specific context, it belongs in a personal note, not in the playbook amend.

### Measuring the loop

`jira-service-management-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `JIRA_SERVICE_MANAGEMENT_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
jira-service-management-cli feedback "the --since flag is inclusive but docs say exclusive"
jira-service-management-cli feedback --stdin < notes.txt
jira-service-management-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `JIRA_SERVICE_MANAGEMENT_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `JIRA_SERVICE_MANAGEMENT_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
jira-service-management-cli profile save briefing --json
jira-service-management-cli --profile briefing servicedesk list
jira-service-management-cli profile list --json
jira-service-management-cli profile show briefing
jira-service-management-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 1 | Other error (e.g. `request note` refusing a note JSM stored as public) |
| 5 | API error (upstream issue; also a missing SITE, CLOUD_ID or WORKSPACE_ID variable) |
| 6 | Partial failure |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `jira-service-management-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add jira-service-management-mcp --env JIRA_SERVICE_MANAGEMENT_SITE=acme --env JIRA_SERVICE_MANAGEMENT_EMAIL=you@example.com --env JIRA_SERVICE_MANAGEMENT_API_TOKEN=<api-token> -- jira-service-management-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which jira-service-management-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   jira-service-management-cli request timeline SD-123 --agent
   ```
4. If ambiguous, drill into subcommand help: `jira-service-management-cli request --help`.
