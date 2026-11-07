---
name: blumira
description: "Every Blumira finding, detection, and agent across your direct org and every MSP sub-account  -  in one offline-searchable store with cross-account triage and over-time trends no single API call can answer. Trigger phrases: `blumira findings`, `triage blumira across accounts`, `what changed in blumira since yesterday`, `blumira detection coverage`, `blumira domain controller exposure`, `blumira MTTR report`, `use blumira`, `run blumira`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Blumira"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - blumira-cli
    install:
      - kind: script
        bins: [blumira-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blumira/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blumira/install.ps1
---

# Blumira - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `blumira-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blumira/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blumira/install.ps1 | iex
   ```
3. Verify: `blumira-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `blumira-cli` and `blumira-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Blumira's API and the community MCP return per-account, point-in-time snapshots. This CLI syncs findings, evidence, detection rules, and agent devices from your org and every MSP client account into a local SQLite store, so you get one ranked cross-account triage queue (triage), what-changed-since-last-sync (drift), MTTR and velocity trends (velocity), SLA aging (sla), detection coverage drift vs the MSP basis (coverage), and domain-controller exposure (exposure). It also mints and auto-refreshes its own JWT from your Client ID and Secret instead of making you bring your own.

## When to Use This CLI

Use this CLI when an agent or analyst needs Blumira security data across more than one client account or across time  -  cross-account triage, what-changed diffs, MTTR/velocity reporting, SLA aging, detection-coverage drift, or domain-controller exposure. Prefer it over raw API calls whenever the question spans accounts (MSP rollup) or spans syncs (trends, drift, re-fire), since those answers do not exist in any single Blumira endpoint.

## Anti-triggers

Do not use this CLI for:
- Raw log or event search (the API exposes findings/rules/agents, not the log stream - use the Blumira console)
- Agent installation or sensor deployment (agents commands are read-only inventory)
- Non-Blumira SIEM/EDR data (use the matching vendor's connector)
- Modifying detection rules (public API is read-only for rules - tune in the Blumira console)

## Unique Capabilities

### Cross-account & over-time intelligence
- **`triage`**  -  One globally-ranked open-findings queue across every MSP client account.

  _Reach for this to start MSP triage: it answers 'what is on fire across all my clients right now' in one ranked list instead of one account at a time._

  ```bash
  blumira-cli triage --priority high --status open --agent
  ```
- **`drift`**  -  New, status-changed, and newly-resolved findings since the last sync, per account.

  _The shift-handoff / morning-standup answer  -  pick this when the user asks what is new or changed, not the full current state._

  ```bash
  blumira-cli drift --since 24h --agent
  ```
- **`velocity`**  -  Mean-time-to-resolve and open-rate per account or overall.

  _Use for reporting and trend tracking  -  it is the only way to get response-time trends Blumira's API does not expose._

  ```bash
  blumira-cli velocity --window 30d --by account --agent
  ```
- **`sla`**  -  Findings about to breach an age-based SLA across all accounts, ranked.

  _Pick this to catch findings slipping past SLA before they breach, instead of discovering breaches after the fact._

  ```bash
  blumira-cli sla --breach-in 4h --priority high --agent
  ```
- **`coverage`**  -  Detection rules missing or disabled in your org versus the MSP basis ruleset.

  _Use to find detection rules that silently drifted away from the MSP basis baseline._

  ```bash
  blumira-cli coverage --against basis --agent
  ```
- **`exposure`**  -  Cross-account agent rollup that flags stale or unprotected domain controllers.

  _Reach for this for billing/coverage reviews and to spot unprotected domain controllers across the whole MSP fleet._

  ```bash
  blumira-cli exposure --flag-dc-stale --agent
  ```
- **`audit`**  -  Findings that were resolved and then re-fired (reopened).

  _Use to catch premature or low-quality resolutions that re-fired._

  ```bash
  blumira-cli audit --min-reopens 2 --agent
  ```
- **`recurring`**  -  The same detection firing repeatedly across accounts and time.

  _Pick this to find the noisy detection or unfixed root cause that keeps generating findings._

  ```bash
  blumira-cli recurring --window 90d --min-count 3 --agent
  ```
- **`overview`**  -  One-screen per-account rollup: open findings by priority, agent coverage, and detection-coverage drift across every client.

  _Run this first each morning: it answers 'which client needs attention' before triage answers 'which finding'._

  ```bash
  blumira-cli overview --agent
  ```
- **`reconcile`**  -  Flat finding-to-org-to-assignee-to-status table for diffing against ConnectWise, Jira, or Zendesk.

  _Use when reconciling Blumira findings against an external ticketing system - one command replaces the manual cross-account export._

  ```bash
  blumira-cli reconcile --status open --csv
  ```
- **`evidence-search`**  -  Full-text search over synced finding evidence to find which findings mention an IOC, hostname, or user.

  _Reach for this during threat hunting when you have an indicator but not a finding ID._

  ```bash
  blumira-cli evidence-search "rdp brute force" --agent
  ```
- **`dc-roster`**  -  Every domain controller across all accounts with last check-in and protected/stale state.

  _Use for fleet and billing reviews needing the full DC inventory; use exposure instead for the prioritized stale/unprotected action list._

  ```bash
  blumira-cli dc-roster --agent
  ```
- **`workload`**  -  Open assigned findings grouped by owner across all accounts, with age buckets.

  _Use when balancing analyst assignments or reviewing team load before sprint or shift planning._

  ```bash
  blumira-cli workload --agent
  ```

### Access that beats the incumbent
- **`auth login`**  -  Mints, caches, and auto-refreshes the Blumira JWT from a Client ID + Secret.

  _Use once to authenticate; every other command then just works without manually exchanging or refreshing tokens._

  ```bash
  blumira-cli auth login --client-id "$BLUMIRA_CLIENT_ID" --client-secret "$BLUMIRA_CLIENT_SECRET"
  ```

## Command Reference

**health**  -  Manage health

- `blumira-cli health`  -  Get API health

**msp**  -  Manage msp

- `blumira-cli msp add-account-finding-comment`  -  Add a comment to a finding for your MSP sub-account
- `blumira-cli msp get`  -  Get a specific MSP sub-account by ID
- `blumira-cli msp get-account-agents-devices`  -  Get or search agents devices for your MSP sub-account
- `blumira-cli msp get-account-agents-keys`  -  Get or search agents keys for your MSP sub-account
- `blumira-cli msp get-account-finding`  -  Get a specific finding by ID for your MSP sub-account
- `blumira-cli msp get-account-finding-comments`  -  Get comments for a specific finding for your MSP sub-account
- `blumira-cli msp get-account-finding-evidence`  -  Returns evidence keys and a paginated first page of evidence rows for the finding in one call.
- `blumira-cli msp get-account-findings`  -  Get or search findings for your MSP sub-account
- `blumira-cli msp get-accounts`  -  Get or search sub-accounts for your MSP
- `blumira-cli msp get-accounts-findings`  -  Get or search findings for all your MSP sub-accounts
- `blumira-cli msp get-agents-device`  -  Get a specific agents device by ID for your MSP sub-account
- `blumira-cli msp get-agents-key`  -  Get a specific agents key by ID for your MSP sub-account
- `blumira-cli msp get-detection-rule-by-account`  -  Get a detection rule by ID for your MSP sub-account
- `blumira-cli msp get-detection-rules-by-account`  -  List detection rules for your MSP sub-account
- `blumira-cli msp get-msp-detection-rule`  -  Get a basis detection rule by ID (for MSP bulk detail)
- `blumira-cli msp get-msp-detection-rules`  -  List basis detection rules (catalog) for MSP bulk
- `blumira-cli msp list-users`  -  Get or search users for your MSP sub-account
- `blumira-cli msp resolve-finding`  -  Resolve a finding for your MSP sub-account
- `blumira-cli msp set-finding-owners`  -  Assign owners to a finding for your MSP sub-account

**org**  -  Manage org

- `blumira-cli org controller-direct-add-comment`  -  Add a comment to a finding in the org
- `blumira-cli org controller-direct-get`  -  Get a specific finding by ID
- `blumira-cli org controller-direct-get-agents-device`  -  Get a specific agents device by ID
- `blumira-cli org controller-direct-get-agents-devices`  -  Get or search agents devices
- `blumira-cli org controller-direct-get-agents-key`  -  Get a specific agents key by ID
- `blumira-cli org controller-direct-get-agents-keys`  -  Get or search agents keys
- `blumira-cli org controller-direct-get-by`  -  Get or search findings for your organization
- `blumira-cli org controller-direct-get-comments`  -  Get comments for a specific finding
- `blumira-cli org controller-direct-get-details`  -  Get details for a specific finding
- `blumira-cli org controller-direct-get-detection-rule`  -  Get a detection rule by ID
- `blumira-cli org controller-direct-get-detection-rules-by`  -  List detection rules for your organization
- `blumira-cli org controller-direct-get-evidence`  -  Returns evidence keys (schema) and a paginated first page of evidence rows for the finding in one call.
- `blumira-cli org controller-direct-list-users`  -  Get or search users for your organization
- `blumira-cli org controller-direct-resolve-finding`  -  Resolve a finding
- `blumira-cli org controller-direct-set-owners`  -  Assign owners to a finding

**resolutions**  -  Manage resolutions

- `blumira-cli resolutions`  -  Get resolution options for findings


**audit**


- `blumira-cli audit`  -  Findings that were resolved and then re-fired (reopened).


**auth**


- `blumira-cli auth login`  -  Mints, caches, and auto-refreshes the Blumira JWT from a Client ID + Secret.


**coverage**


- `blumira-cli coverage`  -  Detection rules missing or disabled in your org versus the MSP basis ruleset.


**dc-roster**


- `blumira-cli dc-roster`  -  Every domain controller across all accounts with last check-in and protected/stale state.


**drift**


- `blumira-cli drift`  -  New, status-changed, and newly-resolved findings since the last sync, per account.


**evidence-search**


- `blumira-cli evidence-search`  -  Full-text search over synced finding evidence to find which findings mention an IOC, hostname, or user.


**exposure**


- `blumira-cli exposure`  -  Cross-account agent rollup that flags stale or unprotected domain controllers.


**overview**


- `blumira-cli overview`  -  One-screen per-account rollup: open findings by priority, agent coverage, and detection-coverage drift across every client.


**reconcile**


- `blumira-cli reconcile`  -  Flat finding-to-org-to-assignee-to-status table for diffing against ConnectWise, Jira, or Zendesk.


**recurring**


- `blumira-cli recurring`  -  The same detection firing repeatedly across accounts and time.


**sla**


- `blumira-cli sla`  -  Findings about to breach an age-based SLA across all accounts, ranked.


**triage**


- `blumira-cli triage`  -  One globally-ranked open-findings queue across every MSP client account.


**velocity**


- `blumira-cli velocity`  -  Mean-time-to-resolve and open-rate per account or overall.


**workload**


- `blumira-cli workload`  -  Open assigned findings grouped by owner across all accounts, with age buckets.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
blumira-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Morning MSP triage

```bash
blumira-cli sync --full && blumira-cli triage --priority high --status open --json
```

Sync every account, then pull the ranked high-priority open queue across all clients as JSON for a dashboard or agent.

### Shift-handoff drift report

```bash
blumira-cli drift --since 12h --json --select account,name,change
```

Show only what changed since the last sync with a narrow field set so an agent does not parse full finding payloads.

### Monthly velocity by account

```bash
blumira-cli velocity --window 30d --by account --csv
```

Export per-account MTTR and open-rate for a 30-day report.

### Find unprotected domain controllers

```bash
blumira-cli exposure --flag-dc-stale --json
```

List domain controllers across all accounts whose agent check-in is stale or missing.

### Hunt an indicator across every client's evidence

```bash
blumira-cli evidence-search "rdp brute force" --agent --select matches.finding_id,matches.name
```

Greps the locally cached evidence corpus plus synced finding payloads for an IOC/hostname/term and returns just the fields an agent needs  -  impossible upstream, where evidence is only retrievable per known finding ID. Pass --fetch to populate the evidence cache live first (bounded by --max-fetch).

## Auth Setup

Blumira uses OAuth2 client-credentials. Generate a Client ID + Client Secret in Settings > Organization > Generate API Credentials, then run `auth login --client-id <id> --client-secret <secret>`  -  the CLI exchanges them at https://auth.blumira.com/oauth/token (audience=public-api) for a ~30-day JWT, caches it, and refreshes it automatically. You can also export a pre-minted token as BLUMIRA_API_TOKEN. Keys are read-write by default; scope them read-only in the Blumira UI for safe read access. MSP tenants address sub-orgs by account_id; nothing is baked in.

Run `blumira-cli doctor` to verify setup.

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
  blumira-cli health --agent
  ```
- **Previewable**  -  `--dry-run` shows the request without sending. A completed dry run exits 0 with `"success": true` and `"dry_run": true` and no HTTP `status`; validation failures still exit non-zero with `"success": false`.
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

- Use `--home <dir>` for one invocation, or set `BLUMIRA_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `BLUMIRA_CONFIG_DIR`, `BLUMIRA_DATA_DIR`, `BLUMIRA_STATE_DIR`, `BLUMIRA_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `BLUMIRA_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `blumira-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "blumira": {
        "command": "blumira-mcp",
        "env": {
          "BLUMIRA_HOME": "/srv/blumira"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `BLUMIRA_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `BLUMIRA_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are cached for five minutes under the cache dir. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `BLUMIRA_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `BLUMIRA_DATA_SOURCE` sets the `--data-source` default.

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
blumira-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "blumira-cli learnings confirm 12"] }
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
       materially more, record the divergence via `blumira-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `blumira-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `blumira-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
blumira-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
blumira-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
blumira-cli teach-playbook \
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
blumira-cli playbook amend \
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

`blumira-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `BLUMIRA_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
blumira-cli feedback "the --since flag is inclusive but docs say exclusive"
blumira-cli feedback --stdin < notes.txt
blumira-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `BLUMIRA_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `BLUMIRA_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
blumira-cli profile save briefing --json
blumira-cli --profile briefing health
blumira-cli profile list --json
blumira-cli profile show briefing
blumira-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `blumira-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add blumira-mcp -- blumira-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `blumira-mcp --transport http` requires `BLUMIRA_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `BLUMIRA_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `BLUMIRA_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which blumira-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   blumira-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `blumira-cli <command> --help`.
