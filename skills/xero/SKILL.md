---
name: xero
description: "Every Xero Accounting read plus a local SQLite ledger  -  aging, reconciliation, and GL tie-out that no other Xero tool computes offline. Trigger phrases: `xero aging report`, `who owes us money in xero`, `reconcile xero payments`, `tie out the general ledger`, `what changed in xero since`, `use xero`, `run xero-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Xero"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - xero-cli
    install:
      - kind: script
        bins: [xero-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/xero/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/xero/install.ps1
---

# Xero  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `xero-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/xero/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/xero/install.ps1 | iex
   ```
3. Verify: `xero-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `xero-cli` and `xero-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

A Go CLI for the Xero Accounting API across invoices, contacts, accounts, payments, bank transactions, items, and the immutable journals feed. It syncs the org into a queryable local store, then answers the questions the web UI, SDKs, and endpoint-mirroring MCP servers cannot in a single call: AR/AP aging (`aging`), payment and bank reconciliation gaps (`reconcile`, `bank-recon`), general-ledger-to-invoice tie-out (`tie-out`), and a running-balance ledger walk (`ledger`).

## When to Use This CLI

Use this CLI when an agent or operator needs to read and analyze a Xero organisation's accounting state from the terminal: pulling invoices/contacts/accounts/payments/bank-transactions/items/journals, or answering portfolio questions like AR aging, payment reconciliation gaps, and general-ledger tie-out. It is the right tool when the question spans more than one entity or should run offline against a synced store rather than re-hitting the rate-limited API.

## Unique Capabilities

### Receivables & exposure
- **`aging`**  -  Bucket outstanding invoices into current / 1-30 / 31-60 / 61-90 / 90+ days overdue with totals per bucket, computed locally from your synced ledger.

  _Reach for this to answer 'who owes us and how overdue' in one offline call instead of paging the web UI per org._

  ```bash
  xero-cli aging --json
  ```
- **`exposure`**  -  Rank contacts by total outstanding amount due across their authorised invoices, with an overdue split and invoice count; drill into one contact for a running-balance statement.

  _Use to find concentration of receivable risk  -  who owes the most and how overdue  -  before a collections push._

  ```bash
  xero-cli exposure --json
  ```

### Reconciliation
- **`reconcile`**  -  Cross-join invoices and payments locally to surface AUTHORISED invoices with an amount still due and no (or partial) applied payment, plus payments not fully applied.

  _Pick this when you need the gap between the ledger and applied cash, not a raw list of either._

  ```bash
  xero-cli reconcile --json
  ```
- **`bank-recon`**  -  List bank transactions still flagged unreconciled and match them to invoices and payments by contact and exact amount to suggest likely reconciliation matches.

  _Use before a reconciliation session to see unreconciled spend and its probable invoice/payment matches up front._

  ```bash
  xero-cli bank-recon --json
  ```

### Ledger audit & tie-out
- **`tie-out`**  -  Sum the immutable journal lines by account code and compare the receivable/payable control-account totals against the sum of outstanding invoice amounts, reporting the variance.

  _Reach for this at period close to prove the books tie before signing off  -  variance of zero is the closure signal._

  ```bash
  xero-cli tie-out --json
  ```
- **`ledger`**  -  Replay the immutable journals feed for one account code as an ordered running-balance statement (date, journal number, debit, credit, running balance), entirely offline.

  _Use to audit exactly what posted to a single account without paging the read-only Journals view in the web UI._

  ```bash
  xero-cli ledger 200 --json
  ```

### Agent-native plumbing
- **`since`**  -  List records across every synced entity whose last-modified timestamp is newer than a given time, read entirely from the local store's sync cursor with zero API calls.

  _Pick this when an agent needs the org delta cheaply instead of re-pulling everything against the rate limit._

  ```bash
  xero-cli since 2026-05-01 --json
  ```
- **`snapshot`**  -  One offline call returning total receivable and payable outstanding, overdue count, unreconciled bank-transaction count, and per-table sync staleness as a single structured object.

  _Reach for this as an agent's single state-of-the-org call instead of fanning out across multiple list endpoints._

  ```bash
  xero-cli snapshot --json
  ```

## Command Reference

**accounts**  -  Manage accounts

- `xero-cli accounts create`  -  Creates a new chart of accounts
- `xero-cli accounts delete`  -  Deletes a chart of accounts
- `xero-cli accounts get`  -  Retrieves the full chart of accounts
- `xero-cli accounts get-accountid`  -  Retrieves a single chart of accounts by using a unique account Id
- `xero-cli accounts update`  -  Updates a chart of accounts

**bank-transactions**  -  Manage bank transactions

- `xero-cli bank-transactions create`  -  Creates one or more spent or received money transaction
- `xero-cli bank-transactions get`  -  Retrieves any spent or received money transactions
- `xero-cli bank-transactions get-banktransactions`  -  Retrieves a single spent or received money transaction by using a unique bank transaction Id
- `xero-cli bank-transactions update`  -  Updates a single spent or received money transaction
- `xero-cli bank-transactions update-or-create`  -  Updates or creates one or more spent or received money transaction

**contacts**  -  Manage contacts

- `xero-cli contacts create`  -  Creates multiple contacts (bulk) in a Xero organisation
- `xero-cli contacts get`  -  Retrieves all contacts in a Xero organisation
- `xero-cli contacts get-by-number`  -  Retrieves a specific contact by contact number in a Xero organisation
- `xero-cli contacts get-contactid`  -  Retrieves a specific contacts in a Xero organisation using a unique contact Id
- `xero-cli contacts update`  -  Updates a specific contact in a Xero organisation
- `xero-cli contacts update-or-create`  -  Updates or creates one or more contacts in a Xero organisation

**invoices**  -  Manage invoices

- `xero-cli invoices create`  -  Creates one or more sales invoices or purchase bills
- `xero-cli invoices get`  -  Retrieves sales invoices or purchase bills
- `xero-cli invoices get-invoiceid`  -  Retrieves a specific sales invoice or purchase bill using a unique invoice Id
- `xero-cli invoices update`  -  Updates a specific sales invoices or purchase bills
- `xero-cli invoices update-or-create`  -  Updates or creates one or more sales invoices or purchase bills

**items**  -  Manage items

- `xero-cli items create`  -  Creates one or more items
- `xero-cli items delete`  -  Deletes a specific item
- `xero-cli items get`  -  Retrieves items
- `xero-cli items get-itemid`  -  Retrieves a specific item using a unique item Id
- `xero-cli items update`  -  Updates a specific item
- `xero-cli items update-or-create`  -  Updates or creates one or more items

**journals**  -  Manage journals

- `xero-cli journals get`  -  Retrieves journals
- `xero-cli journals get-by-number`  -  Retrieves a specific journal using a unique journal number.
- `xero-cli journals get-journalid`  -  Retrieves a specific journal using a unique journal Id.

**payments**  -  Manage payments

- `xero-cli payments create`  -  Creates a single payment for invoice or credit notes
- `xero-cli payments create-endpoint`  -  Creates multiple payments for invoices or credit notes
- `xero-cli payments delete`  -  Updates a specific payment for invoices and credit notes
- `xero-cli payments get`  -  Retrieves payments for invoices and credit notes
- `xero-cli payments get-paymentid`  -  Retrieves a specific payment for invoices and credit notes using a unique payment Id


**aging**


- `xero-cli aging`  -  Bucket outstanding invoices into current / 1-30 / 31-60 / 61-90 / 90+ days overdue with totals per bucket, computed locally from your synced ledger.


**bank-recon**


- `xero-cli bank-recon`  -  List bank transactions still flagged unreconciled and match them to invoices and payments by contact and exact amount to suggest likely reconciliation matches.


**exposure**


- `xero-cli exposure`  -  Rank contacts by total outstanding amount due across their authorised invoices, with an overdue split and invoice count; drill into one contact for a running-balance statement.


**ledger**


- `xero-cli ledger`  -  Replay the immutable journals feed for one account code as an ordered running-balance statement (date, journal number, debit, credit, running balance), entirely offline.


**reconcile**


- `xero-cli reconcile`  -  Cross-join invoices and payments locally to surface AUTHORISED invoices with an amount still due and no (or partial) applied payment, plus payments not fully applied.


**since**


- `xero-cli since`  -  List records across every synced entity whose last-modified timestamp is newer than a given time, read entirely from the local store's sync cursor with zero API calls.


**snapshot**


- `xero-cli snapshot`  -  One offline call returning total receivable and payable outstanding, overdue count, unreconciled bank-transaction count, and per-table sync staleness as a single structured object.


**tie-out**


- `xero-cli tie-out`  -  Sum the immutable journal lines by account code and compare the receivable/payable control-account totals against the sum of outstanding invoice amounts, reporting the variance.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
xero-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Monday AR sweep

```bash
xero-cli aging --json
```

Buckets every outstanding invoice by days overdue so you can build a chase list without exporting and pivoting in a spreadsheet.

### Narrow a verbose invoice list for an agent

```bash
xero-cli invoices list --agent --select Invoices.InvoiceNumber,Invoices.Contact.Name,Invoices.AmountDue,Invoices.Status
```

Xero invoice payloads are large and deeply nested; dotted-path --select returns only the fields an agent needs, keeping context small.

### Find the cash-application gap

```bash
xero-cli reconcile --json
```

Cross-joins invoices and payments locally to show authorised invoices with an amount still due and no applied payment.

### Prove the books tie at close

```bash
xero-cli tie-out --json
```

Compares the immutable general-ledger control accounts against the outstanding invoice totals and reports the variance.

### Audit one account's postings

```bash
xero-cli ledger 200 --json
```

Replays the journal feed for account code 200 as a running-balance statement from the local store.

## Auth Setup

Xero requires two credentials on every call: an OAuth2 Bearer access token and a required Xero-Tenant-Id header. Set XERO_ACCESS_TOKEN (obtained via the OAuth2 authorization-code flow or a Xero Custom Connection client-credentials grant) and XERO_TENANT_ID (the target organisation's tenant id from GET /connections). Run `doctor` to confirm both are present before syncing.

Run `xero-cli doctor` to verify setup.

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
  xero-cli accounts get --agent --select AccountID,AddToWatchlist,BankAccountNumber
  ```
- **Previewable**  -  `--dry-run` shows the request without sending. A completed dry run exits 0 with `"success": true` and `"dry_run": true` and no HTTP `status`; validation failures still exit non-zero with `"success": false`.
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

- Use `--home <dir>` for one invocation, or set `XERO_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `XERO_CONFIG_DIR`, `XERO_DATA_DIR`, `XERO_STATE_DIR`, `XERO_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `XERO_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `xero-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "xero": {
        "command": "xero-mcp",
        "env": {
          "XERO_HOME": "/srv/xero"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `XERO_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `XERO_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are cached for five minutes under the cache dir. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `XERO_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `XERO_DATA_SOURCE` sets the `--data-source` default.

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
xero-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "xero-cli learnings confirm 12"] }
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
       materially more, record the divergence via `xero-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `xero-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `xero-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
xero-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
xero-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
xero-cli teach-playbook \
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
xero-cli playbook amend \
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

`xero-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `XERO_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
xero-cli feedback "the --since flag is inclusive but docs say exclusive"
xero-cli feedback --stdin < notes.txt
xero-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `XERO_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `XERO_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
xero-cli profile save briefing --json
xero-cli --profile briefing accounts get
xero-cli profile list --json
xero-cli profile show briefing
xero-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `xero-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add xero-mcp -- xero-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `xero-mcp --transport http` requires `XERO_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), binds `127.0.0.1:7777` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `XERO_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `XERO_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which xero-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   xero-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `xero-cli <command> --help`.
