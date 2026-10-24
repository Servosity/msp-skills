---
name: acronis
description: "The first real CLI for the Acronis Cyber Protect Cloud platform  -  every tenant, agent, and usage metric mirrored locally, with cross-tenant rollups no single API call returns. Trigger phrases: `acronis backup health across tenants`, `find offline acronis agents`, `acronis usage and billing report`, `which acronis customers aren't protected`, `acronis agent version compliance`, `use acronis cyber protect`, `run acronis-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Acronis"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - acronis-cli
---

# Acronis Cyber Protect Cloud - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `acronis-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/acronis/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/acronis/install.ps1 | iex
   ```
3. Verify: `acronis-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `acronis-cli` and `acronis-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Manage your whole MSP estate from one Go binary: tenants, users, agents, offering items, usage, billing reports, tasks, and activities  -  all synced to a local SQLite store. Then answer questions the Acronis console can't: which customers' backups failed last night (health), which agents went silently offline (agents stale), and where you're billing for protection that isn't running (coverage --unprotected).

## When to Use This CLI

Use this CLI when you manage Acronis Cyber Protect Cloud as an MSP and need answers across many customer tenants at once  -  fleet backup health, offline-agent sweeps, agent-version compliance after a rollout, usage/billing reconciliation at month-end. It is also the right tool for scripting Acronis provisioning and reporting without re-implementing token exchange, pagination, and the datacenter-host dance in every script.

## Unique Capabilities

### Cross-tenant rollups
- **`health`**  -  See backup success / failure / stale across your entire book of customer tenants in one table.

  _Reach for this when an agent or tech needs the one-screen 'which customers' backups failed last night' answer the Acronis console can't give across tenants._

  ```bash
  acronis-cli health --agent
  ```
- **`agents stale`**  -  List backup agents that haven't checked in within a threshold, across every tenant, sorted by customer.

  _Use this to catch silently-offline agents before the customer calls  -  the most common MSP backup-failure root cause._

  ```bash
  acronis-cli agents stale --older-than 7d --agent
  ```
- **`alerts repeat`**  -  Rank resources and tenants by how many distinct days in a window had a failed or missed backup.

  _Use this to separate one-off backup blips from chronically-failing resources that need real remediation._

  ```bash
  acronis-cli alerts repeat --days 14 --agent
  ```
- **`failures`**  -  Flat list of every failed or missed backup task across all tenants in a recent window, newest first.

  _Reach for this when the question is 'show me each backup that failed last night' as individual actionable rows, not rollup counts._

  ```bash
  acronis-cli failures --since 24h --agent
  ```
- **`freshness`**  -  Time since the last successful backup per tenant, flagged against an SLA threshold  -  including tenants never backed up.

  _Reach for this when an agent needs SLA breach detection: which customers have gone too long without a good backup._

  ```bash
  acronis-cli freshness --sla 48h --breached --agent
  ```
- **`customer`**  -  One cross-resource snapshot of a single customer: tenant record, users, licenses, usage, agents, and 7-day backup outcomes joined.

  _Reach for this before a customer call: the full per-customer picture in one command instead of six console drill-downs._

  ```bash
  acronis-cli customer TENANT_ID --agent
  ```

### Billing & licensing
- **`reconcile usages`**  -  Flag usage with no matching offering item and offering items with zero usage, per tenant.

  _Reach for this at month-end to catch under-billing (usage with no SKU) and waste (paid SKUs with zero usage) before invoices go out._

  ```bash
  acronis-cli reconcile usages --tenant <tenant_id> --agent
  ```
- **`coverage`**  -  Surface tenants that pay for protection but have no online agent or no recent successful backup.

  _Use this to find the highest-liability customers  -  billed for backup, not actually protected._

  ```bash
  acronis-cli coverage --unprotected --agent
  ```
- **`usages drift`**  -  Compare per-tenant, per-metric usage between two stored snapshots to see what grew or shrank.

  _Reach for this to explain month-over-month invoice changes and spot runaway storage growth early._

  ```bash
  acronis-cli usages drift --from 2026-04-01 --to 2026-05-01 --agent
  ```
- **`tenants offering-items inventory`**  -  Estate-wide rollup of which offering items and editions are enabled, with per-SKU tenant counts.

  _Reach for this for license trueups and edition migrations: which SKUs are deployed where, in one table._

  ```bash
  acronis-cli tenants offering-items inventory --agent
  ```

### Fleet posture
- **`agents compliance`**  -  Show the distribution of agent versions across the estate and flag tenants behind the target version.

  _Use this after a release rollout to confirm every customer's agents updated, for security and support consistency._

  ```bash
  acronis-cli agents compliance --target 16.0 --agent
  ```
- **`tree`**  -  Render the Partner -> Customer -> Folder -> Unit hierarchy with per-node agent and user counts.

  _Reach for this to understand the shape of a partner's book of business at a glance._

  ```bash
  acronis-cli tree --depth 3
  ```
- **`tenants audit`**  -  Flag enabled customer tenants missing users, offering items, agents, or OAuth clients  -  onboarding drift in one table.

  _Reach for this after onboarding waves: catches half-provisioned tenants before they become missed-backup tickets._

  ```bash
  acronis-cli tenants audit --agent
  ```

## Command Reference

**agent-manager**  -  Manage agent manager

- `acronis-cli agent-manager delete-agent`  -  Cancel registration of a specific Acronis agent.
- `acronis-cli agent-manager delete-agents`  -  Cancel registration and delete service accounts for multiple agents.
- `acronis-cli agent-manager force-agent-update`  -  Launch a forced agent update bypassing maintenance windows for specified agents.
- `acronis-cli agent-manager get-agent`  -  Retrieve details about a specific registered Acronis agent.
- `acronis-cli agent-manager get-agent-update-settings`  -  Fetch update configuration settings for agents or tenants.
- `acronis-cli agent-manager get-hardware-node`  -  Retrieve specific hardware node information including storage configuration.
- `acronis-cli agent-manager list-agents`  -  List all registered Acronis protection agents visible from a specified tenant.
- `acronis-cli agent-manager list-hardware-nodes`  -  List all hardware nodes visible from a specified tenant.
- `acronis-cli agent-manager update-agent-update-settings`  -  Store or update agent update configuration settings including maintenance windows.

**clients**  -  OAuth2 client credential management

- `acronis-cli clients create`  -  Create a new OAuth2 client credential for API authentication.
- `acronis-cli clients delete`  -  Delete an OAuth2 client credential.
- `acronis-cli clients get`  -  Retrieve details about a specific OAuth2 client.
- `acronis-cli clients list`  -  List OAuth2 client credentials registered in the system.

**idp**  -  Manage idp

- `acronis-cli idp request-token`  -  Request an OAuth2 access token using client credentials, authorization code, or other grant types.
- `acronis-cli idp revoke-token`  -  Revoke an OAuth2 access or refresh token.

**remote_search**  -  Manage remote search

- `acronis-cli remote-search`  -  Search for tenants and users by name, email, or login across the accessible hierarchy.

**reports**  -  Manage reports

- `acronis-cli reports`  -  Create a scheduled or on-demand usage report configuration.

**task-manager**  -  Manage task manager

- `acronis-cli task-manager get-activity`  -  Retrieve details about a specific task activity by ID.
- `acronis-cli task-manager get-task`  -  Retrieve details about a specific backup or protection task by ID.
- `acronis-cli task-manager list-activities`  -  Fetch a list of task activities with filtering and pagination.
- `acronis-cli task-manager list-tasks`  -  Fetch a list of backup and protection tasks with filtering, ordering, and pagination support.

**tenants**  -  Tenant hierarchy management and configuration

- `acronis-cli tenants create`  -  Create a new tenant as a child of an existing tenant.
- `acronis-cli tenants delete`  -  Delete a tenant. The tenant must have no children or active services.
- `acronis-cli tenants get`  -  Retrieve details about a specific tenant by ID.
- `acronis-cli tenants list`  -  List tenants in the hierarchy. Can filter by parent tenant UUID or retrieve by specific UUIDs.
- `acronis-cli tenants update`  -  Update tenant properties including name, contact, and enabled status.


**agents**


- `acronis-cli agents stale`  -  List backup agents that haven't checked in within a threshold, across every tenant, sorted by customer.

- `acronis-cli agents compliance`  -  Show the distribution of agent versions across the estate and flag tenants behind the target version.


**alerts**


- `acronis-cli alerts repeat`  -  Rank resources and tenants by how many distinct days in a window had a failed or missed backup.


**coverage**


- `acronis-cli coverage`  -  Surface tenants that pay for protection but have no online agent or no recent successful backup.


**customer**


- `acronis-cli customer`  -  One cross-resource snapshot of a single customer: tenant record, users, licenses, usage, agents, and 7-day backup outcomes joined.


**failures**


- `acronis-cli failures`  -  Flat list of every failed or missed backup task across all tenants in a recent window, newest first.


**freshness**


- `acronis-cli freshness`  -  Time since the last successful backup per tenant, flagged against an SLA threshold  -  including tenants never backed up.


**health**


- `acronis-cli health`  -  See backup success / failure / stale across your entire book of customer tenants in one table.


**reconcile**


- `acronis-cli reconcile usages`  -  Flag usage with no matching offering item and offering items with zero usage, per tenant.


**tenants**


- `acronis-cli tenants audit`  -  Flag enabled customer tenants missing users, offering items, agents, or OAuth clients  -  onboarding drift in one table.

- `acronis-cli tenants offering-items inventory`  -  Estate-wide rollup of which offering items and editions are enabled, with per-SKU tenant counts.


**tree**


- `acronis-cli tree`  -  Render the Partner -> Customer -> Folder -> Unit hierarchy with per-node agent and user counts.


**usages**


- `acronis-cli usages drift`  -  Compare per-tenant, per-metric usage between two stored snapshots to see what grew or shrank.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
acronis-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Monday morning fleet triage

```bash
acronis-cli sync && acronis-cli health --agent --select tenant_id,failed,stale
```

Sync the estate then narrow the health rollup to just the failing and stale columns per tenant.

### Offline-agent sweep into a ticket

```bash
acronis-cli agents stale --older-than 3d --json --select tenant_id,hostname,last_seen | jq '.[]'
```

List agents silent for 3+ days with only the fields a ticket needs.

### Month-end billing reconciliation

```bash
acronis-cli reconcile usages --tenant <tenant_id> --agent
```

Flag usage with no matching SKU and SKUs with zero usage before invoicing.

### Post-rollout version audit

```bash
acronis-cli agents compliance --target 16.0 --json
```

Confirm every tenant's agents reached the target version after a release.

### Estate shape at a glance

```bash
acronis-cli tree --depth 3
```

Render the partner/customer/folder/unit hierarchy with per-node agent and user counts.

## Auth Setup

Acronis uses OAuth2 client credentials. Register an API client in the Acronis console to get a client_id, client_secret, and your datacenter region, then run `acronis-cli auth login`  -  it exchanges them at /api/2/idp/token for a JWT (valid 2 hours) and stores it. Tokens last 2 hours; re-run `auth login` to refresh. The datacenter host is per-partner; set it with --datacenter or ACRONIS_DATACENTER (e.g. us-cloud, eu2-cloud). You can also skip login and provide a JWT directly via ACRONIS_BEARER_AUTH or `auth set-token`.

Run `acronis-cli doctor` to verify setup.

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
  acronis-cli clients list --agent --select client_id,type,status
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

- Use `--home <dir>` for one invocation, or set `ACRONIS_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `ACRONIS_CONFIG_DIR`, `ACRONIS_DATA_DIR`, `ACRONIS_STATE_DIR`, `ACRONIS_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `ACRONIS_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `acronis-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "acronis": {
        "command": "acronis-mcp",
        "env": {
          "ACRONIS_HOME": "/srv/acronis"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `ACRONIS_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `ACRONIS_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads of tenants, clients, agents, tasks and activities in the default `--data-source auto` mode are written to the SQLite store, each in its own endpoint's table; GET responses are cached for five minutes under the cache dir. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `ACRONIS_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `ACRONIS_DATA_SOURCE` sets the `--data-source` default.

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
acronis-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "acronis-cli learnings confirm 12"] }
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
       materially more, record the divergence via `acronis-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `acronis-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `acronis-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
acronis-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
acronis-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
acronis-cli teach-playbook \
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
acronis-cli playbook amend \
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

`acronis-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `ACRONIS_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
acronis-cli feedback "the --since flag is inclusive but docs say exclusive"
acronis-cli feedback --stdin < notes.txt
acronis-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `ACRONIS_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `ACRONIS_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
acronis-cli profile save briefing --json
acronis-cli --profile briefing clients list
acronis-cli profile list --json
acronis-cli profile show briefing
acronis-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `acronis-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

Install the MCP binary from this CLI's published public-library entry or pre-built release, then register it:

```bash
claude mcp add acronis-mcp -- acronis-mcp
```

Verify: `claude mcp list`

For a remote host, `acronis-mcp --transport http` requires `ACRONIS_MCP_HTTP_TOKEN` (callers send `Authorization: Bearer <token>`), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Typed endpoint tools that delete or replace (DELETE, PUT) carry the MCP `destructiveHint` annotation, and those that create or trigger (POST, such as `agent-manager_force-agent-update` and `idp_revoke-token`) are marked not read-only but not destructive. The server asks for no confirmation on either: that step is your MCP host's. `ACRONIS_MCP_READ_ONLY=1` removes every tool not marked read-only.

## Direct Use

1. Check if installed: `which acronis-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   acronis-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `acronis-cli <command> --help`.
