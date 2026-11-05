---
name: gradient
description: "The Synthesize partner API from the terminal - every endpoint, plus a usage-push ledger, billing drift detection, and alert-to-ticket tracing no other Gradient tool has. Trigger phrases: `push usage counts to gradient`, `sync usage to synthesize`, `check my synthesize integration status`, `find unmapped gradient accounts`, `did my gradient alert create a ticket`, `use gradient`, `run gradient-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Gradient MSP"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - gradient-cli
    install:
      - kind: script
        bins: [gradient-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/gradient/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/gradient/install.ps1
---

# Gradient MSP - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `gradient-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/gradient/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/gradient/install.ps1 | iex
   ```
3. Verify: `gradient-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `gradient-cli` and `gradient-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Gradient MSP's Synthesize platform reconciles vendor usage against MSP billing, but its PowerShell SDK makes you write a script project per integration. This CLI covers the full vendor API surface agent-natively with an offline SQLite mirror of your accounts, and adds what the API itself cannot: a local ledger of every count you push (usage drift), debounce-aware bulk pushes (usage push), mapping-hygiene rollups (hygiene unmapped), and async alert-to-ticket confirmation (alert send --wait).

## When to Use This CLI

Use this CLI for any agent task that touches Gradient MSP's Synthesize vendor API: pushing usage unit counts for billing reconciliation, creating or updating accounts and services, checking or flipping integration status, dispatching alerts that must become PSA tickets, and auditing what was pushed (drift, ledgers, unmapped rollups). It is the right choice for nightly usage sync jobs and integration bring-up debugging.

## Anti-triggers

Do not use this CLI for:
- Pulling usage data OUT of vendor products (RMM, security tools) - this CLI only pushes into Synthesize; fetch from the vendor's own API first
- Operating the MSP-side Synthesize web app (mapping approvals happen in the Gradient UI, not this API)
- Managing PSA tickets directly (ConnectWise/Autotask/Halo operations) - alerts here only enqueue tickets via Gradient
- Gradient's internal partner program administration or anything requiring a Synthesize web login session

## Unique Capabilities

### Billing you can audit
- **`usage push`**  -  Push a whole file of unit counts (CSV or JSON) in one shot and trigger exactly one billing rebuild instead of one per call.

  _Reach for this whenever more than one unit count needs to land in Synthesize; it is the nightly sync primitive._

  ```bash
  gradient-cli usage push --file ./counts.csv --agent
  ```
- **`usage drift`**  -  See exactly which accounts' usage changed between your last two pushes, with old-to-new deltas.

  _Run before every invoice cycle to know what moved - the reconciliation pre-flight no API call can answer._

  ```bash
  gradient-cli usage drift --agent
  ```

### Alert-to-ticket correlation
- **`alert send`**  -  Dispatch an alert and wait until the PSA ticket actually exists, instead of fire-and-forget.

  _Use when an alert must verifiably become a ticket - CI hooks and monitoring bridges need the confirmation, not the enqueue._

  ```bash
  gradient-cli alert send --account 123456789 --title "Backup failure" --description "Nightly job failed" --wait --agent
  ```
- **`alert trace`**  -  Review every alert you have dispatched - messageId, account, time, last-known ticket state - and filter to ones that never became tickets.

  _The fastest answer to 'which of my alerts are stuck in the queue' across days of dispatches._

  ```bash
  gradient-cli alert trace --stuck --agent
  ```

### Mapping hygiene
- **`hygiene unmapped`**  -  One rollup of every unmapped account and every service without a vendor SKU, shaped as an actionable work queue.

  _Run it after every account/service push to see what mapping work remains before billing can reconcile._

  ```bash
  gradient-cli hygiene unmapped --agent
  ```
- **`status ready`**  -  A single go/no-go verdict on whether the integration is ready to flip to active: live status, last sync age, and outstanding mapping gaps.

  _Run before 'integration update-status --status active' to avoid activating an integration with unmapped accounts._

  ```bash
  gradient-cli status ready --agent
  ```

## Command Reference

**accounts**  -  Manage accounts

- `gradient-cli accounts create`  -  Creates an account
- `gradient-cli accounts list`  -  Get Organization Accounts
- `gradient-cli accounts update`  -  Updates 1 or more fields per Account
- `gradient-cli accounts update-one <accountId>`  -  Updates 1 or more fields of a single Account

**alerting**  -  Manage alerting

- `gradient-cli alerting <accountId>`  -  Adds an alert to the alerting queue

**billing**  -  Manage billing

- `gradient-cli billing <serviceId>`  -  Set route for adding new unit count for one account, and one service.

**clients**  -  Manage clients

- `gradient-cli clients`  -  DEPRECATED - This route is used to get the Client data needed to handling mapping externally.

**integration**  -  Manage integration

- `gradient-cli integration get`  -  Preferred route for checking credentials
- `gradient-cli integration update-status`  -  Update the integration status for a partner. Set as pending when ready for inital mapping.

**mappings**  -  Manage mappings

- `gradient-cli mappings create`  -  Create Service for a Vendor for an Organization
- `gradient-cli mappings update <serviceId>`  -  Updates 1 or more fields of a single Service
- `gradient-cli mappings update-bulk`  -  Updates 1 or more fields for each Service

**services**  -  Manage services

- `gradient-cli services create`  -  Create Service for a Vendor
- `gradient-cli services get <serviceId>`  -  Retrieves Vendor with VendorSku[] filtered by service id

**ticket_events**  -  Manage ticket events

- `gradient-cli ticket-events <messageId>`  -  Checks for a ticket Event with the provided messageId to have created a ticket in the PSA

**vendor**  -  Manage vendor

- `gradient-cli vendor get`  -  Get Vendor Details
- `gradient-cli vendor update`  -  Update details for a vendor


**alert**


- `gradient-cli alert send`  -  Dispatch an alert and wait until the PSA ticket actually exists, instead of fire-and-forget.

- `gradient-cli alert trace`  -  Review every alert you have dispatched - messageId, account, time, last-known ticket state - and filter to ones that never became tickets.


**hygiene**


- `gradient-cli hygiene unmapped`  -  One rollup of every unmapped account and every service without a vendor SKU, shaped as an actionable work queue.


**status**


- `gradient-cli status ready`  -  A single go/no-go verdict on whether the integration is ready to flip to active: live status, last sync age, and outstanding mapping gaps.


**usage**


- `gradient-cli usage push`  -  Push a whole file of unit counts (CSV or JSON) in one shot and trigger exactly one billing rebuild instead of one per call.

- `gradient-cli usage drift`  -  See exactly which accounts' usage changed between your last two pushes, with old-to-new deltas.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
gradient-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Nightly usage sync with one billing rebuild

```bash
gradient-cli usage push --file ./nightly-counts.csv --agent
```

Validates and fans out every account-service count with no_build until the final call, then records the run in the local ledger.

### What changed since the last push?

```bash
gradient-cli usage drift --agent
```

Joins the two most recent ledger runs and returns only the rows whose counts moved - the pre-invoice sanity check.

### Inspect the vendor SKU catalog fields agents care about

```bash
gradient-cli vendor get --agent --select data.name,data.skus.name,data.skus.category
```

The vendor profile nests the full SKU catalog; --select narrows the payload to the fields that matter.

### Dispatch an alert and confirm the ticket

```bash
gradient-cli alert send --account 123456789 --title "Backup failure" --description "Nightly job failed twice" --wait
```

Dispatches, captures the messageId, then polls the debug route until the PSA ticket exists or timeout.

### Pre-activation readiness check

```bash
gradient-cli status ready --agent
```

Combines live integration status with local mapping gaps into a single go/no-go before 'integration update-status --status active'.

## Auth Setup

Synthesize issues a Vendor API key and a Partner API key pair (Synthesize UI: Integrations -> Custom -> Generate API Tokens; shown once). Every request sends a GRADIENT-TOKEN header whose value is base64("<vendorKey>:<partnerKey>"). Set GRADIENT_VENDOR_API_KEY and GRADIENT_PARTNER_API_KEY and the CLI derives the header for you, or set GRADIENT_TOKEN to the pre-computed value directly. Verify with `gradient-cli doctor`.

## Credential safety

- The `GRADIENT-TOKEN` header and every API request go only to the scheme, host and port of `GRADIENT_BASE_URL` (default `https://app.usegradient.com/api`). A redirect to any other host is refused, so the credential never reaches it.
- Any output, file or MCP result that would repeat `GRADIENT_TOKEN`, the vendor or partner API key, the header derived from them, or a credential-named value from a response prints "[text withheld: it repeated a secret value this program holds]" in its place. Responses are never cached on disk, credential-named fields are dropped before anything reaches the local store, and the push and alert ledgers pass the same withholding.
- Only `credentials.toml` (mode 0600) holds a credential; `auth set-token` reads the token from stdin, never from an argument.
- `--dry-run` sends nothing. Over MCP, a tool that creates or changes Synthesize data (accounts, services, mappings, billing counts, alerts, integration status, vendor profile, usage push, import) returns a preview and sends only with `confirm: true`, and a tool argument can never name a file to read or write (`usage push --file` is CLI-only).

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
  gradient-cli accounts list --agent --select description,id,isMapped
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit confirmation**  -  `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
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

- Use `--home <dir>` for one invocation, or set `GRADIENT_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `GRADIENT_CONFIG_DIR`, `GRADIENT_DATA_DIR`, `GRADIENT_STATE_DIR`, `GRADIENT_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `GRADIENT_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `gradient-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "gradient": {
        "command": "gradient-mcp",
        "env": {
          "GRADIENT_HOME": "/srv/gradient"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `GRADIENT_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `GRADIENT_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are never cached on disk. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `GRADIENT_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `GRADIENT_DATA_SOURCE` sets the `--data-source` default.

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
gradient-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "gradient-cli learnings confirm 12"] }
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
       materially more, record the divergence via `gradient-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `gradient-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `gradient-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
gradient-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
gradient-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
gradient-cli teach-playbook \
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
gradient-cli playbook amend \
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

`gradient-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `GRADIENT_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
gradient-cli feedback "the --since flag is inclusive but docs say exclusive"
gradient-cli feedback --stdin < notes.txt
gradient-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `GRADIENT_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `GRADIENT_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
gradient-cli profile save briefing --json
gradient-cli --profile briefing accounts list
gradient-cli profile list --json
gradient-cli profile show briefing
gradient-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `gradient-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add gradient-mcp -- gradient-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `gradient-mcp --transport http` requires `GRADIENT_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `GRADIENT_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `GRADIENT_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which gradient-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   gradient-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `gradient-cli <command> --help`.
