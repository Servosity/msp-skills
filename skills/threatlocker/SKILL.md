---
name: threatlocker
description: "Every ThreatLocker Portal API feature, plus the write operations the read-only tools lack and a cross-tenant offline store no other ThreatLocker tool has. Trigger phrases: `triage threatlocker approvals`, `approve this hash across all tenants`, `export the threatlocker audit log`, `which threatlocker agents are offline`, `why is threatlocker returning 401`, `use threatlocker`, `run threatlocker`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "ThreatLocker"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - threatlocker-cli
    install:
      - kind: script
        bins: [threatlocker-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatlocker/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatlocker/install.ps1
---

# ThreatLocker  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `threatlocker-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatlocker/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatlocker/install.ps1 | iex
   ```
3. Verify: `threatlocker-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer places the `threatlocker-cli` and `threatlocker-mcp` binaries on your PATH. It does not
register anything with your agent - see [mcp-install.md](./mcp-install.md) for the
MCP wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

A single CLI for MSPs running ThreatLocker across many customer tenants. It matches the full read surface of the incumbent MCP server, adds the writes nobody shipped (approve requests, toggle maintenance, push policy), and mirrors every entity into a local SQLite database so you can triage approvals, audit drift, and device health across ALL tenants at once  -  something the per-tenant API forces you to do one header-swap at a time.

## When to Use This CLI

Use this CLI when you operate ThreatLocker across multiple customer organizations and need to act, not just read  -  clearing approval backlogs at scale, exporting audit evidence before it ages off, finding unhealthy agents, or pushing policy across tenants. It is the right tool when the portal's one-tenant-at-a-time UI or the read-only MCP server is the bottleneck.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Cross-tenant intelligence

- **`approvals triage`**  -  One ranked queue of every pending application approval across all your managed customer tenants, grouped by file hash so duplicate requests collapse into one row.

  _Reach for this to clear the morning approval backlog across an entire MSP book without swapping tenant context request-by-request._

  ```bash
  threatlocker-cli approvals triage --all-tenants --agent
  ```
- **`audit drift`**  -  One ranked table of security-relevant changes  -  protection disabled, policy changed, maintenance toggled  -  across every tenant in a time window.

  _Use this for the weekly compliance sweep or right after a customer reports something changed unexpectedly._

  ```bash
  threatlocker-cli audit drift --since 7d --all-tenants --agent
  ```
- **`devices health`**  -  Joins computers, online-devices, and last-checkin data to classify every endpoint healthy / offline / stale / isolated, rolled up per tenant.

  _Reach for this for the daily 'which agents are dark across all customers' sweep and post-patch verification._

  ```bash
  threatlocker-cli devices health --all-tenants --agent
  ```

### MSP write operations

- **`approvals approve-batch`**  -  Approve the same file (by SHA256) across every tenant where it is pending, in one command, with a dry-run plan first.

  _Use this when one trusted updater is blocked everywhere  -  approve it once instead of clicking through 30 portals._

  ```bash
  threatlocker-cli approvals approve-batch --hash e3b0c44298fc1c149afbf4c8996fb924... --all-tenants --dry-run
  ```

### Audit & compliance

- **`audit export`**  -  Export the Unified Audit log per-tenant or across all tenants to JSONL/CSV and persist it locally, keeping evidence past ThreatLocker's 31-day retention cliff.

  _Run this on a schedule so compliance evidence and incident timelines survive the retention window._

  ```bash
  threatlocker-cli audit export --all-tenants --since 2026-04-01 --agent
  ```
- **`audit retention-check`**  -  Reports, per tenant, the oldest audit row you have versus the 31-day cliff and how stale your last export is  -  flagging tenants about to lose evidence.

  _Pick this to catch a broken export before the data it should have captured ages off the 31-day window forever._

  ```bash
  threatlocker-cli audit retention-check --agent
  ```

### Auth resilience

- **`doctor`**  -  Diagnoses the #1 ThreatLocker integration pain: validates the raw 64-hex token format, the no-Bearer Authorization header, the ManagedOrganizationId header, New-vs-Old API mode, pings an authenticated endpoint, and maps a 401 to its exact likely cause.

  _Run this first whenever a script starts returning 401  -  it tells you whether the token expired, the org header is missing, or you're on the deprecated API mode._

  ```bash
  threatlocker-cli doctor --agent
  ```

## Command Reference

**application-files**  -  File rules belonging to an application

- `threatlocker-cli application-files`  -  List the file rules within an application (paginated)

**applications**  -  Application definitions (custom + built-in) and policies' targets

- `threatlocker-cli applications create`  -  Create a custom application definition
- `threatlocker-cli applications get`  -  Get a single application by id
- `threatlocker-cli applications match`  -  Match a file (hash/path/cert) to existing applications  -  used in the approval flow
- `threatlocker-cli applications research`  -  ThreatLocker security research details (risk ratings, categories, remediation)
- `threatlocker-cli applications search`  -  Search applications (paginated). searchBy: app/full/process/hash/cert/created/categories/countries.
- `threatlocker-cli applications update`  -  Update an application's name/description

**approvals**  -  Application-control approval requests (list, inspect, approve)

- `threatlocker-cli approvals approve`  -  Approve (permit) an application approval request, creating/extending a permit policy. policyLevel: org/group/computer.
- `threatlocker-cli approvals count`  -  Count of pending approval requests
- `threatlocker-cli approvals get`  -  Get a single approval request
- `threatlocker-cli approvals list`  -  List approval requests. statusId 1=Pending,4=Approved,10=Ignored,13=Escalated. Use --child-orgs to span tenants.
- `threatlocker-cli approvals permit-options`  -  Get the permit options for an approval request (inputs to approve)
- `threatlocker-cli approvals storage`  -  Get storage-control approval request details

**audit**  -  Unified Audit (ActionLog)  -  permit/deny events. Default retention 31 days.

- `threatlocker-cli audit file-history`  -  All audit events for a given file path
- `threatlocker-cli audit get`  -  Get a single audit entry by id
- `threatlocker-cli audit search`  -  Search the Unified Audit log. actionId 1=Permit,2=Deny,99=AnyDeny. Requires startDate/endDate.

**computer-groups**  -  Computer groups

- `threatlocker-cli computer-groups dropdown`  -  Simple group dropdown (label/value)
- `threatlocker-cli computer-groups list`  -  List computer groups with nested computers

**computers**  -  Manage and inspect protected computers/devices

- `threatlocker-cli computers baseline-rescan`  -  Restart Baseline (learning) on computers
- `threatlocker-cli computers checkins`  -  Connection/check-in history for a computer (paginated)
- `threatlocker-cli computers delete`  -  Delete/remove computers by id
- `threatlocker-cli computers enable-protection`  -  Enable Secured Mode (re-enable protection) on computers
- `threatlocker-cli computers get`  -  Get a single computer's detail by id
- `threatlocker-cli computers install-info`  -  Deployment/install info for adding new computers
- `threatlocker-cli computers list`  -  List/search computers (paginated). searchBy 1-5; orderBy e.g. computername.
- `threatlocker-cli computers maintenance`  -  Enable maintenance mode (disable protection) on computers for a window
- `threatlocker-cli computers maintenance-update`  -  Set/extend maintenance mode on a single computer
- `threatlocker-cli computers move-org`  -  Move computers to another organization (tenant)
- `threatlocker-cli computers restart-service`  -  Restart the ThreatLocker service on computers

**maintenance**  -  Maintenance-mode history

- `threatlocker-cli maintenance`  -  Maintenance-mode history for a computer (paginated)

**network-policies**  -  Network Control (network access) policies

- `threatlocker-cli network-policies get`  -  Get a single network access policy by id
- `threatlocker-cli network-policies list`  -  List network access policies (paginated)

**online-devices**  -  Currently-online devices

- `threatlocker-cli online-devices`  -  List currently-online devices (paginated)

**organizations**  -  Managed (child) organizations  -  MSP tenants

- `threatlocker-cli organizations auth-key`  -  Get the installation auth key for the current organization
- `threatlocker-cli organizations for-move`  -  List organizations available as computer-move targets
- `threatlocker-cli organizations list`  -  List child/managed organizations (paginated)

**policies**  -  Application Control / Storage / Network policies

- `threatlocker-cli policies copy`  -  Copy policies from a source org/group to target org(s)  -  cross-tenant cloning
- `threatlocker-cli policies create`  -  Create a policy. policyActionId 1=Permit,2=Deny,6=Permit+Ringfence.
- `threatlocker-cli policies delete`  -  Delete policies by id
- `threatlocker-cli policies deploy`  -  Queue a policy deployment for an organization
- `threatlocker-cli policies get`  -  Get a single policy by id
- `threatlocker-cli policies list-by-app`  -  List policies that target an application (paginated)

**reports**  -  Reports

- `threatlocker-cli reports data`  -  Fetch dynamic data for a report
- `threatlocker-cli reports list`  -  List report categories and their reports

**scheduled-actions**  -  Scheduled agent actions

- `threatlocker-cli scheduled-actions get`  -  Get a single scheduled action by id
- `threatlocker-cli scheduled-actions list`  -  List scheduled agent actions
- `threatlocker-cli scheduled-actions search`  -  Search scheduled actions (paginated)

**storage-policies**  -  Storage Control policies

- `threatlocker-cli storage-policies get`  -  Get a single storage policy by id
- `threatlocker-cli storage-policies list`  -  List storage policies (paginated)

**system-audit**  -  Portal system audit (admin actions) + Health Center

- `threatlocker-cli system-audit health-center`  -  Health Center data for the last N days (1-365)
- `threatlocker-cli system-audit search`  -  Search portal admin/system audit entries. Requires startDate/endDate.

**tags**  -  Tags

- `threatlocker-cli tags dropdown`  -  Tag dropdown options (label/value)
- `threatlocker-cli tags get`  -  Get a single tag (with its values) by id

**versions**  -  ThreatLocker agent versions

- `threatlocker-cli versions`  -  List available agent versions (label/value/isEnabled/isDefault/osType)


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
threatlocker-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes


### Morning cross-tenant approval sweep

```bash
threatlocker-cli approvals triage --all-tenants --agent --select organizationName,fileName,hash,ageHours,duplicateCount
```

Drain the overnight backlog ranked by age with duplicate hashes collapsed, then batch-approve the trusted ones.

### Nightly audit archive before the 31-day cliff

```bash
threatlocker-cli audit export --all-tenants --since 2026-04-01 --csv > audit-archive.csv
```

Persist Unified Audit beyond ThreatLocker's retention window for SIEM and compliance.

### Who disabled protection this week

```bash
threatlocker-cli audit drift --since 7d --all-tenants --agent
```

One ranked table of protection-off / policy-change / maintenance events across every customer.

### Dark-agent health sweep

```bash
threatlocker-cli devices health --all-tenants --agent --select organizationName,computerName,healthClass,lastCheckin
```

Classify every endpoint healthy/offline/stale/isolated across all tenants in one pass.

### Diagnose a broken automation

```bash
threatlocker-cli doctor --agent
```

Pinpoint whether a 401 is an expired token, a missing org header, or Old-API mode.

## Auth Setup

Auth is a raw API token in the Authorization header (NO 'Bearer' prefix)  -  a 64-character lowercase hex string created in the portal under Administrators > API Users > Generate API Token. Most calls also need a ManagedOrganizationId header (your tenant GUID); set THREATLOCKER_ORG_ID or pass --org. Tokens renew on each use and silently expire when idle, so run `doctor` if you hit a 401.

Run `threatlocker-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  threatlocker-cli application-files --application-id 550e8400-e29b-41d4-a716-446655440000 --agent --select applicationFileId,applicationId,fullPath
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success

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

- Use `--home <dir>` for one invocation, or set `THREATLOCKER_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `THREATLOCKER_CONFIG_DIR`, `THREATLOCKER_DATA_DIR`, `THREATLOCKER_STATE_DIR`, `THREATLOCKER_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `THREATLOCKER_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `threatlocker-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "threatlocker": {
        "command": "threatlocker-mcp",
        "env": {
          "THREATLOCKER_HOME": "/srv/threatlocker"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `THREATLOCKER_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `THREATLOCKER_HOME`, or `doctor` will not find credentials left under the former root.

## Automatic learning

This CLI ships a self-capturing learning loop. The CLI does its own bookkeeping: every invocation is journaled locally, a failed flag followed by a corrected retry auto-derives a `flag_alias` candidate, and a `teach` on a query family without a playbook auto-synthesizes a `playbook_candidate` from the session's journal. Your job is judgment only: `recall` first, act on surfaced candidates, `teach` the final answer, `playbook amend` when you observe a correction. You never record failures by hand.

### Step 1: `recall` before any discovery

Before list/search/drill commands on a new user question, run:

```bash
threatlocker-cli recall "<user's question>" --agent
```

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
      "next_action": ["<trial command>", "threatlocker-cli learnings confirm 12"] }
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
       materially more, record the divergence via `threatlocker-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `threatlocker-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `threatlocker-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately:

```bash
threatlocker-cli teach --query "<user's question>" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
threatlocker-cli teach \
  --query "<user's question>" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
threatlocker-cli teach-playbook \
  --query "<user's question>" \
  --playbook-file ~/playbooks/<shape>.json \
  --notes-file ~/playbooks/<shape>-notes.md
```

Playbook files are JSON with `steps`, `entity_slots`, `expected_tool_calls`. Notes files are markdown carrying the gotchas verbatim. File-free callers (MCP-only agents) pass the same content inline: `--playbook-json` and `--playbook-notes` on the integrated `teach` form, `--playbook-json` and `--notes` on `teach-playbook`. On the integrated `teach` form, the playbook flags are optional - omit them entirely for a resource-only teach. On the standalone `teach-playbook` form, at least one of the playbook and notes flags must be set; both empty is rejected. Playbooks are keyed on the structural query family (entities stripped) so a recipe taught from one entity-shaped query applies to every other query of the same shape, with `slots_resolved` binding the live query's canonical at recall time.

When you DO find a playbook on a future recall, treat it as ground truth: replay the steps with `slots_resolved` substitutions, skip the discovery that the choreography already documents, and read `notes` before any step.

### Step 6: `playbook amend &` when your debug response identifies a correction

If your debug-protocol response identifies a concrete correction the notes or playbook should know  -  a workaround, an undocumented endpoint shape, a stale field name, observed schema drift, an empty-payload fallback  -  fire `playbook amend` BEFORE emitting your user-facing response. Same fire-and-forget posture as `teach`.

```bash
threatlocker-cli playbook amend \
  --query "<exact recall query string>" \
  --add-note "<your concrete correction>"
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

`threatlocker-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `THREATLOCKER_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
threatlocker-cli feedback "the --since flag is inclusive but docs say exclusive"
threatlocker-cli feedback --stdin < notes.txt
threatlocker-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `THREATLOCKER_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `THREATLOCKER_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
threatlocker-cli profile save briefing --json
threatlocker-cli --profile briefing application-files --application-id 550e8400-e29b-41d4-a716-446655440000
threatlocker-cli profile list --json
threatlocker-cli profile show briefing
threatlocker-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `threatlocker-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add threatlocker-mcp -- threatlocker-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which threatlocker-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   threatlocker-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `threatlocker-cli <command> --help`.
