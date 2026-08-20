---
name: avanan
description: "Every Avanan (Check Point Harmony Email and Collaboration) API operation as one CLI and MCP server, plus shift-start triage, phishing campaign clustering, single-message lifecycle timelines, one exception lookup across all seven security engines, exception conflict auditing, and cross-tenant MSP fleet rollups that a stateless API mirror cannot answer in a single call. Trigger phrases: `check avanan for phishing`, `what did harmony email catch today`, `quarantine this email`, `restore that quarantined message`, `is this domain allowlisted in avanan`, `show me our avanan tenants`, `which avanan tenant is over its seat count`, `use avanan`, `run avanan`."
author: "Abhi Saini"
license: "Apache-2.0"
vendor: "Avanan"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - avanan-cli
    install:
      - kind: go
        bins: [avanan-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/avanan/cmd/avanan-cli
---

# Avanan  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `avanan-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install avanan --cli-only
   ```
2. Verify: `avanan-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.5 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/avanan/cmd/avanan-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Avanan (Check Point Harmony Email & Collaboration) has integrations for XSOAR, Sentinel, n8n, and MCP, but no CLI and nothing that keeps state. This one covers the whole documented surface, including the sectool exception families the published spec omits, and adds a local SQLite mirror that makes triage, campaign, exceptions audit, msp fleet, and timeline possible. It also implements the documented request signature exactly, including the request-string term the docs leave out.

## When to Use This CLI

Use this CLI for Avanan / Check Point Harmony Email & Collaboration work: investigating phishing and malware detections, searching mail and SaaS entities, quarantining or restoring messages, managing allow and block exceptions across all seven policy engines, and running MSP tenant, license, and usage operations. Run `avanan-cli mirror --since 7d` before the offline commands (triage, campaign, timeline, exceptions find, exceptions audit) and `avanan-cli sync --resources msp` before `msp fleet`  -  those commands read the local mirror only and will report empty results against an unpopulated store. It is the right tool when a question spans more than one tenant or more than one point in time, because the mirror answers without spending API quota.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI for Check Point system or audit logs  -  the vendor exposes those through the separate Infinity Portal API, not this one
- Do not use this CLI for Check Point firewall, endpoint, or network security products; it covers email and SaaS collaboration only
- Do not use this CLI to read mail from Microsoft 365 or Google Workspace directly  -  it only sees messages Avanan has scanned
- Do not use this CLI to change Avanan security policy rules or engine configuration; only exceptions are exposed through the API
- Do not use this CLI to reach a tenant in another region  -  credentials are region-scoped and cross-region access is refused by design

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Local state that compounds
- **`triage`**  -  See everything detected in a window you choose, bucketed by threat type, severity, and state, with the sender domains driving the volume.

  _Reach for this at the start of a shift instead of re-querying the same window and eyeballing which detections you already handled._

  ```bash
  avanan-cli triage --since 24h --agent
  ```
- **`campaign`**  -  Group detections into candidate phishing campaigns by sender domain and normalized subject, showing recipient spread and how many are still un-remediated.

  _Reach for this when triage shows volume and you need to know whether forty detections are one campaign or forty problems._

  ```bash
  avanan-cli campaign --since 7d --agent
  ```
- **`timeline`**  -  Reconstruct one message's full history: detection, state changes, actions submitted, task outcomes, and restore disposition, in order.

  _Reach for this when a user disputes a quarantine or a ticket needs evidence of what actually happened to a message._

  ```bash
  avanan-cli timeline f05b74da3ee859eea41aeac40aaad3c2 --agent
  ```

### Collapsing the seven-headed exception system
- **`exceptions find`**  -  Ask whether a domain, sender, URL, or hash is excepted anywhere across all seven engines and nine exception tables at once.

  _Reach for this before adding an allowlist entry, or when a phish got through and you need to know whether one of your own exceptions let it._

  ```bash
  avanan-cli exceptions find partner-domain.com --agent
  ```
- **`exceptions audit`**  -  Flag exceptions that contradict each other across sub-systems, exact duplicates, and entries that have never matched real traffic.

  _Reach for this on a policy review to find the contractor domain nobody removed and the string that is allowlisted in one engine and blocked in another._

  ```bash
  avanan-cli exceptions audit --agent
  ```

### MSP scale
- **`msp fleet`**  -  One ranked table across every tenant joining licensed seats, add-ons, usage, and detection volume.

  _Reach for this to spot tenants over their seat count or with an unusual detection week without walking the tenant list by hand._

  ```bash
  avanan-cli msp fleet --agent
  ```

### Fixing the API's worst footgun
- **`remediate`**  -  Quarantine or restore a batch, wait for the async task to finish, and report the real per-item outcome.

  _Reach for this instead of firing an action and polling by hand, and to turn the single-scope 400 into an error that names your actual scopes._

  ```bash
  avanan-cli remediate quarantine --entity f05b74da3ee859eea41aeac40aaad3c2 --wait --dry-run
  ```

## Command Reference

**action**  -  Manage action

- `avanan-cli action post-entity`  -  Post Entity Action.
- `avanan-cli action post-event`  -  Post Event Action.

**avanan-search**  -  Manage avanan search

- `avanan-cli avanan-search get-saas-entity`  -  Get Saas Entity.
- `avanan-cli avanan-search query-saas-entity`  -  Query Saas Entity.

**download**  -  Manage download

- `avanan-cli download <entity_id>`  -  Download the raw .eml for a SaaS entity.

**download-large-email**  -  Manage download large email

- `avanan-cli download-large-email <entity_id>`  -  Return a presigned URL for downloading a message too large to stream inline.

**event**  -  Manage event

- `avanan-cli event get`  -  Get Event.
- `avanan-cli event query`  -  Query Event.

**exceptions**  -  Manage exceptions

- `avanan-cli exceptions create`  -  Create Ap Exception Blacklist.
- `avanan-cli exceptions create-whitelist`  -  Create Ap Exception Whitelist.
- `avanan-cli exceptions get-ap`  -  Get Ap Exception.
- `avanan-cli exceptions get-ap-exctype`  -  Get Ap Exception.
- `avanan-cli exceptions update`  -  Update Ap Exception Blacklist.
- `avanan-cli exceptions update-whitelist`  -  Update Ap Exception Whitelist.

**msp**  -  Manage msp

- `avanan-cli msp create`  -  Create Msp.
- `avanan-cli msp create-tenants`  -  Create Tenant.
- `avanan-cli msp create-users`  -  Create User.
- `avanan-cli msp delete`  -  Delete Msp.
- `avanan-cli msp delete-tenants`  -  Delete Tenant.
- `avanan-cli msp delete-users`  -  Delete User.
- `avanan-cli msp describe-tenant`  -  Describe Tenant.
- `avanan-cli msp describe-user`  -  Describe User.
- `avanan-cli msp list`  -  List Msps.
- `avanan-cli msp list-addons`  -  List Addons.
- `avanan-cli msp list-daily-usages`  -  List Daily Usages.
- `avanan-cli msp list-licenses`  -  List Licenses.
- `avanan-cli msp list-monthly-usages`  -  List Monthly Usages.
- `avanan-cli msp list-tenants`  -  List Tenants.
- `avanan-cli msp list-users`  -  List Users.
- `avanan-cli msp update`  -  Update User.
- `avanan-cli msp update-or-create-tenant-license`  -  Update Or Create Tenant License.

**report**  -  Manage report

- `avanan-cli report`  -  Report that one or more entities were mis-classified (phishing, spam, clean, or marketing email).

**scopes**  -  Manage scopes

- `avanan-cli scopes`  -  Get Scopes.

**sectool-exceptions**  -  Manage sectool exceptions

- `avanan-cli sectool-exceptions create`  -  Create an exception for the named security engine.
- `avanan-cli sectool-exceptions delete`  -  Delete an exception for the named security engine.
- `avanan-cli sectool-exceptions update`  -  Update an existing exception for the named security engine.

**sectools**  -  Manage sectools

- `avanan-cli sectools create-anomaly-exception`  -  Create an Anomaly engine exception rule.
- `avanan-cli sectools create-ctp-item`  -  Add an entry to a Click-Time Protection exception list.
- `avanan-cli sectools delete-anomaly-exceptions`  -  Delete Anomaly engine exception rules by rule ID.
- `avanan-cli sectools delete-ctp-item`  -  Delete one Click-Time Protection exception entry.
- `avanan-cli sectools delete-ctp-items`  -  Delete multiple Click-Time Protection exception entries by ID.
- `avanan-cli sectools delete-ctp-lists`  -  Delete every Click-Time Protection exception list.
- `avanan-cli sectools get-ctp-item`  -  Get one Click-Time Protection exception entry by ID.
- `avanan-cli sectools get-ctp-list`  -  Get one Click-Time Protection exception list by ID.
- `avanan-cli sectools list-anomaly-exceptions`  -  List the Anomaly engine's exception rules.
- `avanan-cli sectools list-ctp-items`  -  List the individual entries across Click-Time Protection exception lists.
- `avanan-cli sectools list-ctp-lists`  -  List the Click-Time Protection exception lists.
- `avanan-cli sectools update-ctp-item`  -  Update one Click-Time Protection exception entry.
- `avanan-cli sectools update-ctp-items`  -  Update Click-Time Protection exception list entries.

**soar**  -  Manage soar

- `avanan-cli soar get-entity`  -  Retrieve the decoded email record for a SaaS entity, including headers and body.
- `avanan-cli soar post-notify`  -  Send an exposure notification about a specific entity to a list of email addresses.

**task**  -  Manage task

- `avanan-cli task <task_id>`  -  Get Task.


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
avanan-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Shift handoff in one command

```bash
avanan-cli triage --since 12h --agent
```

Requires a populated mirror (`mirror --since 7d`). Everything detected in the window, bucketed by type, severity, and state, with the top offending sender domains and a count of anything still unresolved.

### Narrow a large event query to just what an agent needs

```bash
avanan-cli event query --request-data-event-types phishing --agent --select responseData.eventId,responseData.type,responseData.state,responseData.entityInfo.subject,responseData.entityInfo.senderEmail
```

Avanan event payloads nest entity detail several levels deep and run to tens of KB per call; the dotted --select paths keep only the fields worth reading.

### Check an allowlist request before you grant it

```bash
avanan-cli exceptions find vendor-domain.com --agent
```

Requires `mirror --resources exceptions` first. Searches all nine exception tables at once and shows which engine, which list side, and who created each hit  -  replacing nine separate API calls.

### Quarantine a confirmed campaign and prove it landed

```bash
avanan-cli remediate quarantine --entity f05b74da3ee859eea41aeac40aaad3c2 --wait --dry-run
```

Resolves the required single scope, submits the action, polls the task to a terminal state, and prints the per-item outcome. Drop --dry-run to execute.

### Find tenants running over their licensed seats

```bash
avanan-cli msp fleet --agent --select rows.tenant,rows.seats_licensed,rows.seats_used,rows.detections
```

Requires `sync --resources msp` and `mirror --since 7d` first. One local join across mirrored tenants, licenses, and detection volume, ranked by seat overage.

## Auth Setup

Avanan uses one of two schemes depending on your host, and the CLI picks automatically. On legacy `*.avanan.net` hosts it performs the signed handshake: a fresh request UUID, your application ID, a GMT timestamp, and an `x-av-sig` computed as sha256(base64(reqId + appId + date + requestString + secret)). The resulting token lasts one hour and is refreshed for you. On Infinity Portal hosts it exchanges your access key for a bearer token instead. Set `AVANAN_APP_ID` and `AVANAN_CLIENT_SECRET`, pick your region, and run `auth login`. Credentials are region-scoped: a US key cannot read EU data, by design.

Run `avanan-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  avanan-cli event get mock-value --scopes example-value --x-av-req-id 550e8400-e29b-41d4-a716-446655440000 --agent --select actions,additionalData,availableEventActions
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
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

- Use `--home <dir>` for one invocation, or set `AVANAN_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `AVANAN_CONFIG_DIR`, `AVANAN_DATA_DIR`, `AVANAN_STATE_DIR`, `AVANAN_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `AVANAN_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `avanan-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "avanan": {
        "command": "avanan-mcp",
        "env": {
          "AVANAN_HOME": "/srv/avanan"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `AVANAN_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `AVANAN_HOME`, or `doctor` will not find credentials left under the former root.

## Automatic learning

This CLI ships a self-capturing learning loop. The CLI does its own bookkeeping: every invocation is journaled locally, a failed flag followed by a corrected retry auto-derives a `flag_alias` candidate, and a `teach` on a query family without a playbook auto-synthesizes a `playbook_candidate` from the session's journal. Your job is judgment only: `recall` first, act on surfaced candidates, `teach` the final answer, `playbook amend` when you observe a correction. You never record failures by hand.

### Step 1: `recall` before any discovery

Before list/search/drill commands on a new user question, run:

```bash
avanan-cli recall "<user's question>" --agent
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
      "next_action": ["<trial command>", "avanan-cli learnings confirm 12"] }
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
       materially more, record the divergence via `avanan-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `avanan-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `avanan-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately:

```bash
avanan-cli teach --query "<user's question>" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
avanan-cli teach \
  --query "<user's question>" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
avanan-cli teach-playbook \
  --query "<user's question>" \
  --playbook-file ~/playbooks/<shape>.json \
  --notes-file ~/playbooks/<shape>-notes.md
```

Playbook files are JSON with `steps`, `entity_slots`, `expected_tool_calls`. Notes files are markdown carrying the gotchas verbatim. File-free callers (MCP-only agents) pass the same content inline: `--playbook-json` and `--playbook-notes` on the integrated `teach` form, `--playbook-json` and `--notes` on `teach-playbook`. On the integrated `teach` form, the playbook flags are optional - omit them entirely for a resource-only teach. On the standalone `teach-playbook` form, at least one of the playbook and notes flags must be set; both empty is rejected. Playbooks are keyed on the structural query family (entities stripped) so a recipe taught from one entity-shaped query applies to every other query of the same shape, with `slots_resolved` binding the live query's canonical at recall time.

When you DO find a playbook on a future recall, treat it as ground truth: replay the steps with `slots_resolved` substitutions, skip the discovery that the choreography already documents, and read `notes` before any step.

### Step 6: `playbook amend &` when your debug response identifies a correction

If your debug-protocol response identifies a concrete correction the notes or playbook should know  -  a workaround, an undocumented endpoint shape, a stale field name, observed schema drift, an empty-payload fallback  -  fire `playbook amend` BEFORE emitting your user-facing response. Same fire-and-forget posture as `teach`.

```bash
avanan-cli playbook amend \
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

`avanan-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `AVANAN_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
avanan-cli feedback "the --since flag is inclusive but docs say exclusive"
avanan-cli feedback --stdin < notes.txt
avanan-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `AVANAN_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `AVANAN_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
avanan-cli profile save briefing --json
avanan-cli --profile briefing event get mock-value --scopes example-value --x-av-req-id 550e8400-e29b-41d4-a716-446655440000
avanan-cli profile list --json
avanan-cli profile show briefing
avanan-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `avanan-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/avanan/cmd/avanan-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add avanan-mcp -- avanan-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which avanan-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   avanan-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `avanan-cli <command> --help`.
