---
name: afi
description: "The first CLI for Afi SaaS backup  -  full public-API coverage plus the fleet-wide coverage, staleness, and offboarding answers the rate-limited API can't serve live. Trigger phrases: `check afi backup coverage`, `which mailboxes aren't backed up in afi`, `afi fleet health`, `offboard a user from afi backup`, `afi backup stale check`, `use afi`, `run afi-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Afi"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - afi-cli
    install:
      - kind: script
        bins: [afi-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/afi/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/afi/install.ps1
---

# Afi - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `afi-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/afi/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/afi/install.ps1 | iex
   ```
3. Verify: `afi-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `afi-cli` and `afi-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

An open-source CLI and MCP server for Afi, whose vendor explicitly discourages API polling. This CLI walks the whole hierarchy into local SQLite once (fleet-sync), then answers fleet-wide questions (coverage-gaps, fleet-health, backup-stale, reconcile-licenses) offline. A guarded offboard command runs the vendor's own archive-then-release sequence with a verification gate the portal lacks.

## When to Use This CLI

Use this CLI for any programmatic question about Afi SaaS backup fleets: backup coverage, nightly task health, quota breaches, archive staleness, license reconciliation, and the guarded archive-and-offboard sequence for departing employees. It is the right choice for MSP-scale questions across many tenants, where the portal forces a per-tenant walk and the rate-limited API punishes polling  -  sync once, then query the local store.

## Anti-triggers

Do not use this CLI for:
- Performing restores, exports, or browsing actual backup data  -  the Afi public API does not expose restore/export initiation or data download; use the Afi web portal
- Creating or editing backup policies  -  policies are read-only in the public API; configure them in the portal
- Real-time monitoring or continuous polling of task status  -  Afi explicitly throttles and may suspend polling applications; use scheduled syncs instead
- Managing Afi portal user accounts, SSO, or notification settings  -  not part of the public API surface

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Local state that compounds
- **`fleet-sync`**  -  Walk the whole Afi hierarchy  -  installations, orgs, tenants, then each tenant's resources, protections, policies, archives, quotas, and task stats  -  into local SQLite in one respectful pass.

  _Run this first (and on a schedule): every fleet question afterward  -  coverage, staleness, licensing, scorecards  -  answers offline without tripping Afi's rate limits._

  ```bash
  afi-cli fleet-sync
  ```
- **`coverage-gaps`**  -  Find resources in a tenant that have no backup protection applied  -  the backup blind spots.

  _Reach for this when asked whether every mailbox, site, or drive is actually being backed up  -  it answers fleet-coverage questions one API can't._

  ```bash
  afi-cli coverage-gaps --tenant 01F000000000000411Z1101G1Y --agent
  ```
- **`fleet-health`**  -  One table across all synced tenants: task success/failure counts, failed-task list, and quota breaches.

  _Use this for the Monday-morning 'is the whole fleet green' sweep instead of walking tenants one portal tab at a time._

  ```bash
  afi-cli fleet-health --since 24h --agent
  ```
- **`resolve`**  -  Map a Microsoft 365 / Google Workspace ID, email, or name to the canonical Afi resource or tenant, including Multi-Geo fan-out.

  _Run this before any offboard or audit to be certain which Afi object corresponds to an external-system identity._

  ```bash
  afi-cli resolve jane.doe@example.com --agent
  ```
- **`reconcile-licenses`**  -  Compare purchased subscription quantities against actually-protected resource counts per tenant to flag over- and under-provisioning.

  _Use this for the weekly licensing drift check that today requires manual spreadsheet stitching across two portal screens._

  ```bash
  afi-cli reconcile-licenses --org 01F0ORG0000000000000000000 --json
  ```
- **`backup-stale`**  -  Flag protected resources that are not getting backed up (no archive at all, or no backup task finished in their tenant in the synced window)  -  protected-but-silently-failing resources.

  _Use this to catch the silent-failure class: resources that have a policy attached but whose backups quietly stopped landing._

  ```bash
  afi-cli backup-stale --agent
  ```
- **`tenant-scorecard`**  -  One tenant's full backup posture: coverage percentage, last task status, newest/oldest archive age, and quota state.

  _Generate this card when prepping a customer ticket or QBR  -  the per-tenant deep dive that complements fleet-health._

  ```bash
  afi-cli tenant-scorecard 01F000000000000411Z1101G1Y --agent --select tenant_id,coverage_pct,quotas_exceeded
  ```

### Guarded workflows
- **`offboard`**  -  Safely back up a departing user's resource, verify the archive landed, then release the protection  -  refusing the irreversible step until the backup is confirmed.

  _Use this when offboarding an employee so the final backup is verified before the M365/GWS seat is released  -  never lose a leaver's data._

  ```bash
  afi-cli offboard 01F0RESOURCE0000000000000A --tenant 01F0TENANT00000000000000B --policy 01F0POLICY000000000000000C --reason "employee departure"
  ```

## Command Reference

**applications**  -  Manage applications

- `afi-cli applications`  -  Lists installations for the application identified by the authentication key.

**orgs**  -  Manage orgs

- `afi-cli orgs create`  -  Creates a new child organization.
- `afi-cli orgs get`  -  Retrieves an organization by its ID.

**tenants**  -  Manage tenants

- `afi-cli tenants <id>`  -  Retrieves a tenant by its ID.


**backup-stale**


- `afi-cli backup-stale`  -  Flag protected resources that are not getting backed up (no archive at all, or no backup task finished in their tenant in the synced window)  -  protected-but-silently-failing resources.


**coverage-gaps**


- `afi-cli coverage-gaps`  -  Find resources in a tenant that have no backup protection applied  -  the backup blind spots.


**fleet-health**


- `afi-cli fleet-health`  -  One table across all synced tenants: task success/failure counts, failed-task list, and quota breaches.


**fleet-sync**


- `afi-cli fleet-sync`  -  Walk the whole Afi hierarchy  -  installations, orgs, tenants, then each tenant's resources, protections, policies, archives, quotas, and task stats  -  into local SQLite in one respectful pass.


**offboard**


- `afi-cli offboard`  -  Safely back up a departing user's resource, verify the archive landed, then release the protection  -  refusing the irreversible step until the backup is confirmed.


**reconcile-licenses**


- `afi-cli reconcile-licenses`  -  Compare purchased subscription quantities against actually-protected resource counts per tenant to flag over- and under-provisioning.


**resolve**


- `afi-cli resolve`  -  Map a Microsoft 365 / Google Workspace ID, email, or name to the canonical Afi resource or tenant, including Multi-Geo fan-out.


**tenant-scorecard**


- `afi-cli tenant-scorecard`  -  One tenant's full backup posture: coverage percentage, last task status, newest/oldest archive age, and quota state.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
afi-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Monday fleet sweep

```bash
afi-cli fleet-sync --skip-archives && afi-cli fleet-health --failed-only --agent
```

Refresh the fleet store (skipping the heavy archives walk), then list only the tenants with failed tasks or quota breaches.

### Find backup blind spots in one tenant

```bash
afi-cli coverage-gaps --tenant 01F000000000000411Z1101G1Y --agent --select items.name,items.kind,items.external_id
```

LEFT JOIN of resources against protections, narrowed to the three fields an agent needs to open tickets.

### Offboard a departed employee safely

```bash
afi-cli resolve jane.doe@example.com && afi-cli offboard 01F0RESOURCE0000000000000A --tenant 01F0TENANT00000000000000B --policy 01F0POLICY000000000000000C --reason "employee departure" --dry-run
```

Resolve the ambiguous email to the canonical Afi resource, then preview the guarded final-backup-verify-release sequence before running it for real.

### Catch silently failing backups

```bash
afi-cli backup-stale --json
```

Protected resources with no archive at all, or in a tenant where no backup task finished in the synced window  -  the failure class no portal screen surfaces.

### Licensing drift check

```bash
afi-cli reconcile-licenses --org 01F0ORG0000000000000000000 --json
```

Purchased subscription quantities vs actually-protected resource counts, per tenant.

## Auth Setup

Create an Application and API key in the Afi portal (org level: Configuration → Apps; tenant level: Service → Settings → Apps), then set AFI_API_KEY. The key is sent as the raw Authorization header value (Afi keys look like appkey-...; no Bearer prefix). Each Application supports two keys for rotation. Keys inherit the Application's installation scope  -  if a tenant or org is missing from results, check where the Application is installed.

Run `afi-cli doctor` to verify setup.

## Credential safety

- The API key goes only to the Afi API host. `AFI_API_KEY` (sent raw in the `Authorization` header) goes only to the scheme, host and port of `https://papi.afi.ai`, or of `AFI_BASE_URL` when set. A redirect to any other host is refused, and a request that would carry the key anywhere but that header (a query value, an org name, another header) is refused before it is sent.
- Any output, file or MCP result that would repeat the key, or a credential-named value from a response, prints "[text withheld: it repeated a secret value this program holds]" in its place. Responses are never cached on disk, and credential-named fields are dropped before anything reaches the local store.
- Only `credentials.toml` (mode 0600) holds a credential; `auth set-token` reads the key from stdin, never from an argument.
- `--dry-run` sends nothing. Over MCP, every tool that changes Afi data (`orgs_create`, `tenants_archives_delete`, `tenants_jobs_tasks-trigger`, `tenants_resources_protections-protect`, `tenants_resources_protections-unprotect`, `offboard`) returns a preview and sends only with `confirm: true`, and a tool argument can never name a file to read or write (so `import`, which reads a local file, is CLI-only).

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
  afi-cli orgs get mock-value --agent --select external_id,id,kind
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

- Use `--home <dir>` for one invocation, or set `AFI_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `AFI_CONFIG_DIR`, `AFI_DATA_DIR`, `AFI_STATE_DIR`, `AFI_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `AFI_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `afi-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "afi": {
        "command": "afi-mcp",
        "env": {
          "AFI_HOME": "/srv/afi"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `AFI_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `AFI_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are never cached on disk. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `AFI_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `AFI_DATA_SOURCE` sets the `--data-source` default.

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
afi-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "afi-cli learnings confirm 12"] }
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
       materially more, record the divergence via `afi-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `afi-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `afi-cli sync --resources applications,archives,licensing,orgs,policies,protections,resources,tenants` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
afi-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
afi-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
afi-cli teach-playbook \
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
afi-cli playbook amend \
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

`afi-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `AFI_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
afi-cli feedback "the --since flag is inclusive but docs say exclusive"
afi-cli feedback --stdin < notes.txt
afi-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `AFI_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `AFI_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
afi-cli profile save briefing --json
afi-cli --profile briefing orgs get mock-value
afi-cli profile list --json
afi-cli profile show briefing
afi-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `afi-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add afi-mcp -- afi-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `afi-mcp --transport http` requires `AFI_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `AFI_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `AFI_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which afi-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   afi-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `afi-cli <command> --help`.
