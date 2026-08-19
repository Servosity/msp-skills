---
name: cork
description: "Every Cork API operation as one CLI and MCP server, plus cross-client risk attribution, exploitability-first vulnerability triage, overdue-compliance detection, and stale-connector health checks that a stateless API mirror cannot answer in a single call. Trigger phrases: `which Cork clients got worse this week`, `why did this client's Cork score drop`, `what should we patch first in Cork`, `are we exposed to this CVE`, `which Cork integrations are stale`, `which clients have no cyber warranty`, `use cork`, `run cork`."
author: "Abhi Saini"
license: "Apache-2.0"
vendor: "Cork"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - cork-cli
    install:
      - kind: script
        bins: [cork-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cork/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cork/install.ps1
---

# Cork - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `cork-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cork/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cork/install.ps1 | iex
   ```
3. Verify: `cork-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed with skill commands until verification succeeds.

Cork tells you a client's risk score moved but never why, and offers no way to ask a question across your whole book of business. This CLI mirrors the API into local SQLite, then answers the questions that need history and fan-out: score attribute explains what drove a score change, score regressions ranks every client by how far they slipped, vulnerabilities triage orders patching by what is actually being exploited rather than by CVSS, and integrations health catches the connector that reports healthy while its data quietly went stale.

## When to Use This CLI

Reach for this CLI when the question spans more than one Cork client or more than one point in time. It is the right tool for book-of-business risk triage, building a patch queue ordered by exploitability, auditing which connectors have silently stopped feeding data, and preparing warranty or QBR conversations backed by score history. It is also the right tool when an agent needs Cork data in structured form without paging through a web dashboard.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI to provision distributor partners or push software installs; those endpoints mutate real customer environments and belong in a reviewed change process, not an agent loop
- Do not use `import` in an agent loop for any resource. `import <resource> --input file.jsonl` issues one POST per line into those same endpoints (`import software` -> `/software/installer/install`, `import distributor` -> `/distributor/partners`, `import integrations` -> `/integrations`) and, by its own help text, failed records are logged but do not stop the run. It is the bulk form of the two commands named above and carries their blast radius; preview with `--dry-run` and keep a human on it
- Do not use this CLI to retrieve or relay integration credentials; `integrations credentials get-integration` prints the endpoint's response verbatim with no redaction, and `integrations raw-data get-integration` returns a presigned URL that downloads the connector's full raw data with no further auth. Both are reads whose output IS the secret - keep them out of any blanket allow-all-reads policy
- Do not use this CLI as a substitute for the Cork web platform when configuring integrations or editing notification settings interactively
- Do not use this CLI for security advisories or CVE metadata in general; it only knows about CVEs Cork has already associated with your fleet, so pair it with a vulnerability database for research

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Risk that compounds locally
- **`score attribute`**  -  Explain why a client's Cork score moved, broken out by claims, compliance, coverage, and vulnerability impact.

  _Reach for this when someone asks why a client got worse, instead of pulling raw score points and eyeballing a trend._

  ```bash
  cork-cli score attribute 00000000-0000-0000-0000-000000000000 --since 30d --agent
  ```
- **`score regressions`**  -  Rank every client by score change over a window, worst movers first.

  _Use this to open a book-of-business review; it answers 'who moved backwards' in one call instead of one call per client._

  ```bash
  cork-cli score regressions --since 7d --min-drop 10 --agent
  ```

### Exploitability over CVSS
- **`vulnerabilities triage`**  -  Rank software products by exploitability (known-exploited first, then EPSS, then CVSS) with a blast-radius device count and client names resolved.

  _Pick this over a raw vulnerability list when deciding what to patch first. Live-only: Cork's vulnerability rows carry no id so they cannot be mirrored locally, --limit counts products rather than individual findings, and a capped scan sets scan_cap_hit._

  ```bash
  cork-cli vulnerabilities triage --kev-only --min-epss 0.3 --limit 25 --agent
  ```
- **`vulnerabilities exposure`**  -  List every affected client, device, product, and version for a single CVE id.

  _Use this the moment a specific CVE is named in an advisory or a client email; it is the only way to answer 'are we exposed'. Exit 3 means the scan succeeded and found no exposure, which is a negative answer rather than a failure. Live-only: vulnerability rows carry no id so there is no local mirror to read, and a capped scan sets scan_cap_hit, which is explicitly not a clean bill of health._

  ```bash
  cork-cli vulnerabilities exposure CVE-2026-1234 --agent
  ```

### Data you can actually trust
- **`compliance overdue`**  -  Surface compliance events that have blown their event type's remediation window, bucketed by age.

  _Choose this over listing a client's events when you need the ones that are actively costing score, ordered by how late they are._

  ```bash
  cork-cli compliance overdue --bucket --agent
  ```
- **`integrations health`**  -  Flag connectors that are down, degraded, or reporting healthy while their last sync has gone stale, and name the clients they feed.

  _Run this before trusting any risk number; a silently stale connector makes every downstream score fiction._

  ```bash
  cork-cli integrations health --stale-after 24h --agent
  ```
- **`coverage gaps`**  -  Diff the devices a connector reports against the devices attributed to the client to expose endpoints one tool sees and another misses.

  _Use this during onboarding verification or when coverage impact is dragging a score and you need the specific unmonitored endpoints. Exit 3 means there were no connector devices to diff, a negative answer rather than a failure; the command refuses outright rather than reporting gaps against a device baseline it could not read._

  ```bash
  cork-cli coverage gaps --client 00000000-0000-0000-0000-000000000000 --agent
  ```

### Commercial signal
- **`warranties exposure`**  -  Rank unwarranted or lapsed clients by current risk so coverage conversations start with the ones that need it most.

  _Reach for this when preparing commercial outreach, rather than listing warranties and cross-checking risk by hand._

  ```bash
  cork-cli warranties exposure --limit 20 --agent
  ```

## Command Reference

**clients**  -  Manage clients

- `cork-cli clients`  -  List clients with their financial protection status (`warranty_status`), associated integration tenants

**compliance**  -  Manage compliance

- `cork-cli compliance get-event-notification-settings`  -  List the notification and alerting rules configured for compliance events on client assets.
- `cork-cli compliance get-event-types`  -  List all compliance event types with their descriptions and cure periods.
- `cork-cli compliance get-events`  -  List policy violations and risk events detected for a client's assets.

**distributor**  -  Manage distributor

- `cork-cli distributor get-partners`  -  List partner sub-accounts managed by this distributor.
- `cork-cli distributor provision-partner`  -  Provision a new Partner account in the system. _note: Requires distributor privileges._

**integrations**  -  Manage integrations

- `cork-cli integrations connect`  -  Connect an API-based integration. This creates a new integration that will immediately begin syncing data.
- `cork-cli integrations delete`  -  Delete an integration and stop all data collection from it. _note: Only API-created integrations can be deleted._
- `cork-cli integrations get-available`  -  List integration types that can be connected to Cork, including required credential fields.
- `cork-cli integrations get-connected`  -  List integrations that have been connected to Cork, including their vendor, connection status, and sync details.
- `cork-cli integrations update`  -  Update an API-created integration's display name and/or credentials.

**invoices**  -  Manage invoices

- `cork-cli invoices`  -  List billing invoices. Returns invoice UUIDs required by `Get Invoice Line Items`.

**me**  -  Manage me

- `cork-cli me`  -  Information on the authenticated user

**software**  -  Manage software

- `cork-cli software get-installer-history`  -  List past software install attempts (most recent first) with their dispatch state, target client/device, package
- `cork-cli software get-installer-setup`  -  Get the one-time setup steps for an RMM vendor that requires manual setup before software installs work.
- `cork-cli software get-packages`  -  List software packages available to install across supported package managers (WinGet, Chocolatey).
- `cork-cli software install`  -  Install a software package on a single mapped device via the device's RMM integration.

**vulnerabilities**  -  Manage vulnerabilities

- `cork-cli vulnerabilities get-software`  -  List individual software vulnerabilities with full CVE details including CVSS score, EPSS score, KEV (known exploited)
- `cork-cli vulnerabilities get-software-vulnerability-summary`  -  Get a rollup of CVEs grouped by software product, showing number of impacted devices, impacted versions

**warranties**  -  Manage warranties

- `cork-cli warranties`  -  List active cyber warranty packages.


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
cork-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Explain a score drop before a QBR

```bash
cork-cli score attribute 00000000-0000-0000-0000-000000000000 --since 30d --agent
```

Returns the four score impact components differenced across the window so you can name the cause instead of describing the trend.

### Narrow a deeply nested vulnerability payload for an agent

```bash
cork-cli vulnerabilities get-software --agent --select sw_vendor,sw_product,cves.cve_id,cves.epss,cves.is_kev --page-size 20
```

Vulnerability rows nest a full CVE array per product; selecting dotted paths keeps the exploitability signals and drops the rest of the payload.

### Answer an advisory the moment a CVE is named

```bash
cork-cli vulnerabilities exposure CVE-2026-1234 --agent
```

Scans the live vulnerability collection page by page and matches the CVE locally, because no Cork endpoint accepts a CVE filter. Exit 3 means the scan found no exposure; check scan_cap_hit before reading that as a clean bill of health.

### Verify a new client is fully monitored after onboarding

```bash
cork-cli coverage gaps --client 00000000-0000-0000-0000-000000000000 --agent
```

Diffs connector-reported devices against client-attributed devices to list endpoints that one tool sees and another is missing.

### Find a client by name without knowing its UUID

```bash
cork-cli search "Northwind" --type clients --limit 10
```

Full-text search over the local mirror resolves a human name to the UUID every other command takes.

## Auth Setup

Cork uses a bearer API key. Mint one in the Cork platform under Admin then API Keys, choosing a name and an expiry. Set it as CORK_API_KEY, or store it with cork-cli auth set-token. One caveat worth knowing: an API key inherits the permissions of the user who created it, so a 403 on a distributor or integration endpoint usually means the key was minted by an operator without that scope, not that the key is wrong.

Run `cork-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  cork-cli clients --agent --select associated_tenants,created_at,hidden
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

- Use `--home <dir>` for one invocation, or set `CORK_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `CORK_CONFIG_DIR`, `CORK_DATA_DIR`, `CORK_STATE_DIR`, `CORK_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `CORK_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `cork-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "cork": {
        "command": "cork-mcp",
        "env": {
          "CORK_HOME": "/srv/cork"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `CORK_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `CORK_HOME`, or `doctor` will not find credentials left under the former root.

## Automatic learning

This CLI ships a self-capturing learning loop. The CLI does its own bookkeeping: every invocation is journaled locally, a failed flag followed by a corrected retry auto-derives a `flag_alias` candidate, and a `teach` on a query family without a playbook auto-synthesizes a `playbook_candidate` from the session's journal. Your job is judgment only: `recall` first, act on surfaced candidates, `teach` the final answer, `playbook amend` when you observe a correction. You never record failures by hand.

### Step 1: `recall` before any discovery

Before list/search/drill commands on a new user question, run:

```bash
cork-cli recall "<user's question>" --agent
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
      "next_action": ["<trial command>", "cork-cli learnings confirm 12"] }
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
       materially more, record the divergence via `cork-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `cork-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `cork-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately:

```bash
cork-cli teach --query "<user's question>" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
cork-cli teach \
  --query "<user's question>" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
cork-cli teach-playbook \
  --query "<user's question>" \
  --playbook-file ~/playbooks/<shape>.json \
  --notes-file ~/playbooks/<shape>-notes.md
```

Playbook files are JSON with `steps`, `entity_slots`, `expected_tool_calls`. Notes files are markdown carrying the gotchas verbatim. File-free callers (MCP-only agents) pass the same content inline: `--playbook-json` and `--playbook-notes` on the integrated `teach` form, `--playbook-json` and `--notes` on `teach-playbook`. On the integrated `teach` form, the playbook flags are optional - omit them entirely for a resource-only teach. On the standalone `teach-playbook` form, at least one of the playbook and notes flags must be set; both empty is rejected. Playbooks are keyed on the structural query family (entities stripped) so a recipe taught from one entity-shaped query applies to every other query of the same shape, with `slots_resolved` binding the live query's canonical at recall time.

When you DO find a playbook on a future recall, treat it as ground truth: replay the steps with `slots_resolved` substitutions, skip the discovery that the choreography already documents, and read `notes` before any step.

### Step 6: `playbook amend &` when your debug response identifies a correction

If your debug-protocol response identifies a concrete correction the notes or playbook should know  -  a workaround, an undocumented endpoint shape, a stale field name, observed schema drift, an empty-payload fallback  -  fire `playbook amend` BEFORE emitting your user-facing response. Same fire-and-forget posture as `teach`.

```bash
cork-cli playbook amend \
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

`cork-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `CORK_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
cork-cli feedback "the --since flag is inclusive but docs say exclusive"
cork-cli feedback --stdin < notes.txt
cork-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `CORK_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `CORK_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
cork-cli profile save briefing --json
cork-cli --profile briefing clients
cork-cli profile list --json
cork-cli profile show briefing
cork-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `cork-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server (the same installer ships both binaries):
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cork/install.sh)
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add cork-mcp -- cork-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which cork-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   cork-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `cork-cli <command> --help`.
