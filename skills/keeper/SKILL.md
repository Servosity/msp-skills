---
name: keeper
description: "Read-only Keeper MSP posture across every client, with history, a change feed and no path to a vault secret. Trigger phrases: `which keeper clients have users without 2fa`, `keeper msp posture report`, `keeper seats per managed company`, `keeper breachwatch exposure across clients`, `use keeper`, `run keeper`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Keeper Security"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - keeper-cli
    install:
      - kind: script
        bins: [keeper-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/keeper/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/keeper/install.ps1
---

# Keeper Security Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `keeper-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/keeper/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/keeper/install.ps1 | iex
   ```
3. Verify: `keeper-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `keeper-cli` and `keeper-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Every managed company's 2FA coverage, weak and reused passwords, BreachWatch exposure, inactive users and seats in one run through Keeper Commander Service Mode. Runs are stored locally, so posture diff and events list show what changed since last time. A fixed allowlist of read-only reports is the whole surface: it cannot read a record or change a setting.

## When to Use This CLI

Use this CLI when an MSP needs Keeper admin facts across its managed companies: seats and billing, user rosters, 2FA coverage, password-health scores, BreachWatch exposure, inactive users, audit events, and what changed since the last check. It is read-only by construction and safe to hand to an agent.

## Anti-triggers

Do not use this CLI for:
- Reading, creating or rotating vault records, passwords, attachments or one-time shares: use Keeper Secrets Manager or the Keeper vault
- Adding, removing or changing managed companies, users, roles or seats: use the Keeper Admin Console or Commander directly
- Locking, deleting or transferring users found by a report: act in the Keeper Admin Console after review

## Unique Capabilities

### Cross-client posture and history
- **`posture report`**  -  Fetch 2FA coverage, weak and reused passwords, BreachWatch exposure, inactive users and seat usage for every managed company in one run, and record it.

  _Reach for this when asked how every client is doing on Keeper security posture._

  ```bash
  keeper-cli posture report --days 90 --agent
  ```
- **`posture diff`**  -  Compare two stored posture runs (needs two 'posture report' runs): per-company metric deltas and per-user transitions such as 2FA turned off or newly at risk.

  _Use for a QBR 'since last time' answer without re-reading every client._

  ```bash
  keeper-cli posture diff --agent
  ```
- **`events list`**  -  Read posture change events (user.twofa_disabled, posture.weak_passwords_increased and more) after a cursor, once each.

  _Use to react to posture changes from automation without re-diffing._

  ```bash
  keeper-cli events list --since 0 --agent
  ```
- **`users find`**  -  Find which managed company a user belongs to, with status, 2FA and last login, across every client; reads the newest stored 'posture report' run (run it first).

  _Use when a ticket names only an email address._

  ```bash
  keeper-cli users find jane@example.com --agent
  ```
- **`users hygiene`**  -  List stale invites (with how long they have been pending), locked, inactive and pending-transfer users across every client; reads the newest stored 'posture report' run (run it first).

  _Use for weekly offboarding and cleanup sweeps._

  ```bash
  keeper-cli users hygiene --kind invited,inactive --agent
  ```
- **`users risk`**  -  List users without 2FA (admins outside any 2FA-enforcing role first), BreachWatch at-risk users and users with weak or reused passwords across every client; reads the newest stored 'posture report' run (run it first).

  _Use to answer which clients have users without 2FA._

  ```bash
  keeper-cli users risk --kind no-2fa --agent
  ```
- **`posture runs`**  -  List stored posture runs with their time, status and inactivity window.

  _Use to pick run ids for posture diff._

  ```bash
  keeper-cli posture runs --agent
  ```

### Read-only admin and MSP reports
- **`companies list`**  -  List every managed company with plan, seats allocated and active, storage and add-ons.

  _Use for seat and plan questions across clients._

  ```bash
  keeper-cli companies list --agent
  ```
- **`companies get`**  -  Show one managed company's plan, seats and add-ons by id.

  _Use for one client's licensing._

  ```bash
  keeper-cli companies get 4421 --agent
  ```
- **`products list`**  -  List MSP products, add-ons and file plans with pricing.

  _Use for pricing questions._

  ```bash
  keeper-cli products list --agent
  ```
- **`permits list`**  -  Show which products and add-ons the MSP may allocate and the maximum file plan.

  _Use before quoting a plan change._

  ```bash
  keeper-cli permits list --agent
  ```
- **`billing report`**  -  Show the monthly consumption billing statement, optionally by company and by date.

  _Use for the monthly billing close._

  ```bash
  keeper-cli billing report --month 2026-09 --by-company --agent
  ```
- **`users list`**  -  List an enterprise's or a managed company's users with status, transfer status, node, roles and 2FA.

  _Use for one client's user roster._

  ```bash
  keeper-cli users list --mc 4421 --agent
  ```
- **`users last-login`**  -  Show each user's last login within a window.

  _Use to check one client's activity live._

  ```bash
  keeper-cli users last-login --mc 4421 --days 90 --agent
  ```
- **`nodes list`**  -  List the enterprise node tree with user, team and role counts.

  _Use to understand a tenant's structure._

  ```bash
  keeper-cli nodes list --agent
  ```
- **`teams list`**  -  List teams with node and user count.

  _Use for team membership questions._

  ```bash
  keeper-cli teams list --mc 4421 --agent
  ```
- **`roles list`**  -  List roles with admin flag, default role, node and user count.

  _Use to find admin roles._

  ```bash
  keeper-cli roles list --mc 4421 --agent
  ```
- **`security-audit report`**  -  Show each user's weak, fair, medium, strong, reused and unique password counts, security score and 2FA.

  _Use for one client's password health._

  ```bash
  keeper-cli security-audit report --mc 4421 --agent
  ```
- **`breachwatch summary`**  -  Show each user's BreachWatch at-risk, passed and ignored counts.

  _Use for one client's breach exposure._

  ```bash
  keeper-cli breachwatch summary --mc 4421 --agent
  ```
- **`audit-trail events`**  -  Read raw audit events, newest first, filtered by period, user and event type.

  _Use to investigate who did what._

  ```bash
  keeper-cli audit-trail events --created last_7_days --limit 50 --agent
  ```
- **`audit-trail summary`**  -  Count audit events by type per day, week, month or the whole period.

  _Use for activity trends._

  ```bash
  keeper-cli audit-trail summary --period day --created last_7_days --agent
  ```
- **`allowlist`**  -  Print the read-only Commander commands this CLI can send and the matching Service Mode -c value.

  _Use to configure Service Mode's own allowlist for defense in depth._

  ```bash
  keeper-cli allowlist --agent
  ```

## Command Reference

**health**  -  Check that the Service Mode host is up

- `keeper-cli health`  -  Liveness probe.

**queue**  -  Read the Service Mode request queue

- `keeper-cli queue`  -  Service Mode queue status: the whole queue's size plus active and completed request counts


**allowlist**


- `keeper-cli allowlist`  -  Print the read-only Commander commands this CLI can send and the matching Service Mode -c value.


**audit-trail**


- `keeper-cli audit-trail events`  -  Read raw audit events, newest first, filtered by period, user and event type.

- `keeper-cli audit-trail summary`  -  Count audit events by type per day, week, month or the whole period.


**billing**


- `keeper-cli billing report`  -  Show the monthly consumption billing statement, optionally by company and by date.


**breachwatch**


- `keeper-cli breachwatch summary`  -  Show each user's BreachWatch at-risk, passed and ignored counts.


**companies**


- `keeper-cli companies list`  -  List every managed company with plan, seats allocated and active, storage and add-ons.

- `keeper-cli companies get`  -  Show one managed company's plan, seats and add-ons by id.


**events**


- `keeper-cli events list`  -  Read posture change events (user.twofa_disabled, posture.weak_passwords_increased and more) after a cursor, once each.


**nodes**


- `keeper-cli nodes list`  -  List the enterprise node tree with user, team and role counts.


**permits**


- `keeper-cli permits list`  -  Show which products and add-ons the MSP may allocate and the maximum file plan.


**posture**


- `keeper-cli posture report`  -  Fetch 2FA coverage, weak and reused passwords, BreachWatch exposure, inactive users and seat usage for every managed company in one run, and record it.

- `keeper-cli posture diff`  -  See what changed between two posture runs: per-company metric deltas and per-user transitions such as 2FA turned off or newly at risk.

- `keeper-cli posture runs`  -  List stored posture runs with their time, status and inactivity window.


**products**


- `keeper-cli products list`  -  List MSP products, add-ons and file plans with pricing.


**roles**


- `keeper-cli roles list`  -  List roles with admin flag, default role, node and user count.


**security-audit**


- `keeper-cli security-audit report`  -  Show each user's weak, fair, medium, strong, reused and unique password counts, security score and 2FA.


**teams**


- `keeper-cli teams list`  -  List teams with node and user count.


**users**


- `keeper-cli users find`  -  Find which managed company a user belongs to, with status, 2FA and last login, across every client at once.

- `keeper-cli users hygiene`  -  List stale invites (with how long they have been pending), locked users, inactive users and pending account transfers across every client.

- `keeper-cli users risk`  -  List users without 2FA (admins outside any 2FA-enforcing role first), BreachWatch at-risk users and users with weak or reused passwords across every client.

- `keeper-cli users list`  -  List an enterprise's or a managed company's users with status, transfer status, node, roles and 2FA.

- `keeper-cli users last-login`  -  Show each user's last login within a window.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
keeper-cli which "users without 2fa"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Seats per client, narrowed

```bash
keeper-cli companies list --agent --select company_id,company_name,allocated,active
```

Allocated against active seats for every managed company, only the columns that matter.

### Quarterly posture snapshot

```bash
keeper-cli posture report --days 90 --agent
```

One run across every client; stored for the next diff.

### What changed since last run

```bash
keeper-cli posture diff --agent
```

Per-company deltas and per-user transitions between the two newest runs.

### Admins without 2FA

```bash
keeper-cli users risk --kind no-2fa --admins-only --agent
```

Admin users with 2FA off, across every client.

### Change feed for automation

```bash
keeper-cli events list --since 0 --agent
```

Typed events after a cursor; pass the returned cursor next time.

## Auth Setup

This CLI talks to Keeper Commander Service Mode, which you run on your own host (service-create). Set KEEPER_BASE_URL to that host (for example https://keeper-svc.example.internal:9090; https only, so run Service Mode with TLS: service-create -crtf and -crtp, and KEEPER_CA_CERT trusts a private certificate) and give it the Service Mode API key: export KEEPER_API_KEY, or store it once with 'keeper-cli auth set-token' (reads the key from stdin into the 0600 credential store). 'keeper-cli auth status' shows which credential source is in use without printing it, and 'keeper-cli auth logout' removes the stored key. Commander on that host must be logged in to your Keeper region (US, EU, AU, CA, JP or GOV) as an MSP or enterprise admin. Set the key's own command list to exactly the commands 'allowlist' prints.

Run `keeper-cli doctor` to verify setup.

## Security model

- **Read-only by construction.** The CLI can send only the Commander commands `keeper-cli allowlist` prints (msp-info, msp-billing-report, enterprise-info, user-report, security-audit-report, audit-report, switch-to-mc, switch-to-msp), each with fixed read-only flags. Every value you pass (a managed company id, a day count, a month, an email, an event type) is checked against a strict pattern and sent as one token, so no argument can add a flag or another command; anything else is refused before a request is made.
- **Vault record contents are out of scope.** Nothing here reads records, passwords, attachments, shared-folder secrets or one-time shares: an admin and MSP view does not need them, and leaving them out means an agent with this CLI cannot leak them. Report rows are cut to known fields, and a response that carries anything shaped like record content is refused whole.
- **Defense in depth: set Service Mode's own allowlist.** Create the Service Mode key with exactly the commands above (`service-create -c 'msp-info,msp-billing-report,enterprise-info,user-report,security-audit-report,audit-report,switch-to-mc,switch-to-msp'`, or `service-config-add`), so Service Mode itself refuses anything else even if another tool shares the host.
- **Managed-company context is shared.** Commander's managed-company context is one global per Service Mode process. Reads return the session to the MSP first; `--mc` reads switch in, check Keeper confirmed the switch, read, and switch back. Every run of this CLI for this user on the machine (the CLI and the MCP server alike, whatever `--home` or environment it runs with) holds one lock per Service Mode host for each read, so this CLI's own reads never interleave, and every read checks afterwards that the session is still where it was: a read another client disturbed is discarded with an error, never shown or stored. Run this CLI against a Service Mode instance no other tool is switching at the same time (a separate API key does not isolate it, and another OS user or another machine running this CLI counts as another tool).
- **The MCP server writes almost nothing.** `posture_report` saves its run to the local store, and it returns a preview and sends nothing until it is called with `confirm` set to boolean true. The learning commands (`teach`, `learnings confirm`, `learnings forget`, `playbook amend`) are CLI-only, not MCP tools. Read-only tools run directly and write nothing, not even a usage row.
- **The API key.** `KEEPER_API_KEY` is sent only to `KEEPER_BASE_URL` (scheme, host and port); no redirect is ever followed, and plain `http` is refused on every host, localhost included: run Service Mode with TLS (`service-create -crtf ... -crtp ...`), and trust a private certificate with `KEEPER_CA_CERT`. The key is withheld from every output, log, error, file and MCP result, in every encoding. It is stored only in the 0600 credential store.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, and `--deliver` paths:

- `--json`  -  one JSON document on stdout (notes go to stderr)
- `--compact`  -  accepted for compatibility; every command already returns only its own fields, none is dropped
- `--csv` / `--plain`  -  tabular rows (collection envelopes unwrap to the row array). `--csv` prefixes `'` on a cell or header key that starts with `=`, `+`, `-`, `@`, tab, CR, or LF (fullwidth `＝＋－＠` too) so a spreadsheet does not run it as a formula; a plain signed number such as `-5` or `+2.5e3` is unchanged.
- `--quiet`  -  one identity value per row, no envelope

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  keeper-cli health --agent --select status
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  `posture diff`, `posture runs`, `events list` and `users find`, `users hygiene` and `users risk` read the local SQLite store
- **Non-interactive**  -  never prompts, every input is a flag
- **Read-only**  -  no command changes Keeper: no vault records, and no user, role, team, seat or managed-company changes

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local" | "computed"},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set  -  piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Paths and state

Agents should treat the CLI's path resolver as part of the runtime contract:

- Use `--home <dir>` for one invocation, or set `KEEPER_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `KEEPER_CONFIG_DIR`, `KEEPER_DATA_DIR`, `KEEPER_STATE_DIR`, `KEEPER_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `KEEPER_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml` and the posture store `data.db`. `state` contains the learn journal and `teach.log`. `cache` holds regenerable files; the HTTP response cache is off.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `keeper-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "keeper": {
        "command": "keeper-mcp",
        "env": {
          "KEEPER_HOME": "/srv/keeper"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `KEEPER_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `KEEPER_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Typed reads are not stored and the HTTP response cache is off; only `posture report` writes the SQLite store (posture runs and change events). `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `KEEPER_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `posture report`; `--data-source local` still reads an existing store, and `KEEPER_DATA_SOURCE` sets the `--data-source` default.

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
keeper-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "keeper-cli learnings confirm 12"] }
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
       materially more, record the divergence via `keeper-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `keeper-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
keeper-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
keeper-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
keeper-cli teach-playbook \
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
keeper-cli playbook amend \
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

`keeper-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `KEEPER_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
keeper-cli feedback "the --since flag is inclusive but docs say exclusive"
keeper-cli feedback --stdin < notes.txt
keeper-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `KEEPER_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `KEEPER_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
keeper-cli profile save briefing --json
keeper-cli --profile briefing health
keeper-cli profile list --json
keeper-cli profile show briefing
keeper-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Other error (for example a `--data-source` mismatch, or `posture report` under `KEEPER_NO_LOCAL_DATA=1`) |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `keeper-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add keeper-mcp --env KEEPER_BASE_URL=https://keeper-svc.example.internal:9090 --env KEEPER_API_KEY="$KEEPER_API_KEY" -- keeper-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `keeper-mcp --transport http` requires `KEEPER_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), binds `127.0.0.1` port 7777 by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `KEEPER_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `KEEPER_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own local stores, the learn store and the posture store, stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which keeper-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   keeper-cli users risk --kind no-2fa --agent
   ```
4. If ambiguous, drill into subcommand help: `keeper-cli users --help`.
