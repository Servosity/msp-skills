---
name: knowbe4
description: "Every KnowBe4 KMSAT reporting feature, plus a local SQLite store for the questions the console cannot answer. Trigger phrases: `who keeps clicking phishing in knowbe4`, `knowbe4 repeat clickers`, `knowbe4 risk leaderboard`, `who failed phishing but never trained`, `knowbe4 training coverage gaps`, `build a knowbe4 qbr`, `use knowbe4`, `run knowbe4-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "KnowBe4"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - knowbe4-cli
    install:
      - kind: script
        bins: [knowbe4-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/knowbe4/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/knowbe4/install.ps1
---

# KnowBe4 - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `knowbe4-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/knowbe4/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/knowbe4/install.ps1 | iex
   ```
3. Verify: `knowbe4-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `knowbe4-cli` and `knowbe4-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

knowbe4-cli mirrors the KnowBe4 Reporting API (users, groups, phishing security tests and campaigns, training campaigns and enrollments, policies, and risk-score history) and syncs it all into a local SQLite database. That local store powers transcendence commands no other KnowBe4 tool has: repeat-clickers finds humans who failed multiple phishing tests, untrained-clickers anti-joins phishing failures against training records, risk-drift ranks who deteriorated this quarter, and qbr assembles a full client quarterly review in one shot.

## When to Use This CLI

Use knowbe4-cli when an agent or MSP analyst needs to answer security-awareness-posture questions that span entities or time: who keeps clicking phishing, who failed a test but never trained, whose risk worsened this quarter, which groups drove account risk up, or to assemble a client QBR. It is the right tool whenever the answer requires joining KnowBe4's separate reporting endpoints or diffing risk over time - work the console and a single API call cannot do.

## Anti-triggers

Do not use this CLI for:
- creating or modifying KnowBe4 users, groups, or phishing/training campaigns (the Reporting API surface is read-only)
- launching or scheduling a real phishing test or training enrollment
- any mutation other than pushing/deleting custom user risk events via the events group (separate KNOWBE4_USER_EVENT_API_KEY, --dry-run-previewable)
- mutations outside KnowBe4: ticketing, email, CRM, ordering, purchasing, booking

## Unique Capabilities

### Phishing behavior intelligence
- **`repeat-clickers`**  -  Surface users who clicked the bait in two or more distinct phishing security tests within a window  -  your hardest-to-train humans.

  _Reach for this when an agent needs the riskiest 5% of humans to target for enforced training instead of blasting everyone._

  ```bash
  knowbe4-cli repeat-clickers --min-clicks 2 --since 90d --top 25 --agent
  ```
- **`report-rate`**  -  Rank users and groups by how often they report simulated phish versus falling for them - surfacing who never uses the Phish Alert Button.

  _Reach for this when the question is about reporting behavior (good security instincts), not just failure counts._

  ```bash
  knowbe4-cli report-rate --bottom 25 --agent
  ```

### Risk posture that compounds
- **`risk-drift`**  -  Rank users and groups by how much their risk score worsened (or improved) between a prior snapshot and now.

  _Pick this for QBR/board prep  -  it turns a wall of risk numbers into the short list of who is deteriorating._

  ```bash
  knowbe4-cli risk-drift --window 90d --worsened --top 20 --agent
  ```
- **`phish-prone-trend`**  -  Plot a group's phish-prone percentage across sequential phishing tests to show whether training is actually working.

  _Produces the headline QBR chart ('phish-prone went 28% to 6%') per group from your own synced history._

  ```bash
  knowbe4-cli phish-prone-trend --group "Finance" --since 12mo --agent
  ```
- **`risk-leaderboard`**  -  Highest-risk users ranked, each enriched with click history, report rate, and overdue-training context.

  _Reach for this when a risk number needs context  -  clicked twice, reported never, two trainings past due  -  so the remediation writes itself._

  ```bash
  knowbe4-cli risk-leaderboard --top 25 --agent
  ```
- **`group-risk-contribution`**  -  Attribute account-level risk movement to the specific groups (departments) driving it up or down.

  _Answers 'why did our risk go up' by department instead of hand-waving  -  turning a board question into a targeted action._

  ```bash
  knowbe4-cli group-risk-contribution --window 90d --top 10 --agent
  ```

### Coverage anti-joins
- **`untrained-clickers`**  -  Anti-join: users who clicked a phish but have no Passed training enrollment to show for it.

  _The single most actionable remediation list a vCISO owns: people who proved they're vulnerable and have zero completed training._

  ```bash
  knowbe4-cli untrained-clickers --since 180d --agent
  ```
- **`coverage-gaps`**  -  Find active users enrolled in zero phishing campaigns or zero training campaigns  -  the people your program silently misses.

  _Use when closing compliance holes  -  uncovered users (including unenrolled new hires) are unmeasured risk._

  ```bash
  knowbe4-cli coverage-gaps --type training --joined-within 30d --agent
  ```

### Reporting rituals
- **`qbr`**  -  One command that assembles the full quarterly review: risk trend, phish-prone trend, training completion, and the top-risk humans.

  _Collapses an afternoon of CSV exports and pivots into one command  -  the biggest recurring time win for an MSP._

  ```bash
  knowbe4-cli qbr --since 90d --format md
  ```

### Local state that compounds
- **`freshness`**  -  See per-table sync freshness and whether the cross-entity commands have the data they need - before trusting a clicker hunt.

  _Run this first when results look empty or stale; it names the exact sync command that fixes the gap._

  ```bash
  knowbe4-cli freshness --agent
  ```

## Command Reference

**account**  -  Your KnowBe4 account: subscription, seat count, admins, and org-level risk score

- `knowbe4-cli account info`  -  Get account and subscription data (name, subscription level, seats, admins, current risk score)
- `knowbe4-cli account risk-score-history`  -  Get the org-level risk-score history time series

**groups**  -  Groups/Smart Groups  -  membership counts and group-level risk score

- `knowbe4-cli groups get`  -  Get a single group by id
- `knowbe4-cli groups list`  -  List all groups with member counts and risk scores
- `knowbe4-cli groups members`  -  List the users in a group
- `knowbe4-cli groups risk-score-history`  -  Get a group's risk-score history time series

**phishing-campaigns**  -  Phishing campaigns  -  recurring/one-time simulated-phishing programs

- `knowbe4-cli phishing-campaigns get`  -  Get a single phishing campaign by id
- `knowbe4-cli phishing-campaigns list`  -  List all phishing campaigns
- `knowbe4-cli phishing-campaigns security-tests`  -  List the phishing security tests (PSTs) run by a campaign

**phishing-tests**  -  Phishing security tests (PSTs)  -  individual simulated-phishing sends with click/report stats

- `knowbe4-cli phishing-tests get`  -  Get a single phishing security test by id
- `knowbe4-cli phishing-tests list`  -  List all phishing security tests (PSTs) across every campaign
- `knowbe4-cli phishing-tests recipient`  -  Get a single recipient's result within a PST
- `knowbe4-cli phishing-tests recipients`  -  List per-user results for a PST (who opened, clicked, reported, entered data)

**policies**  -  Uploaded policies  -  policy documents assigned for acknowledgement

- `knowbe4-cli policies get`  -  Get a single policy by id
- `knowbe4-cli policies list`  -  List all uploaded policies

**store-purchases**  -  Store purchases  -  training content/modules licensed to your account

- `knowbe4-cli store-purchases get`  -  Get a single store purchase by id
- `knowbe4-cli store-purchases list`  -  List all training store purchases (licensed content/modules)

**training-campaigns**  -  Training campaigns  -  assigned security-awareness training programs

- `knowbe4-cli training-campaigns get`  -  Get a single training campaign by id
- `knowbe4-cli training-campaigns list`  -  List all training campaigns

**training-enrollments**  -  Training enrollments  -  per-user module assignments with completion status

- `knowbe4-cli training-enrollments get`  -  Get a single training enrollment by id
- `knowbe4-cli training-enrollments list`  -  List all training enrollments with completion status and time spent

**users**  -  Users in the KnowBe4 console  -  phish-prone percentage, current risk score, group membership

- `knowbe4-cli users get`  -  Get a single user by id
- `knowbe4-cli users list`  -  List all users with risk score and phish-prone percentage
- `knowbe4-cli users risk-score-history`  -  Get a user's risk-score history time series


**coverage-gaps**


- `knowbe4-cli coverage-gaps`  -  Find active users enrolled in zero phishing campaigns or zero training campaigns  -  the people your program silently misses.


**freshness**


- `knowbe4-cli freshness`  -  See per-table sync freshness and whether the cross-entity commands have the data they need - before trusting a clicker hunt.


**group-risk-contribution**


- `knowbe4-cli group-risk-contribution`  -  Attribute account-level risk movement to the specific groups (departments) driving it up or down.


**phish-prone-trend**


- `knowbe4-cli phish-prone-trend`  -  Plot a group's phish-prone percentage across sequential phishing tests to show whether training is actually working.


**qbr**


- `knowbe4-cli qbr`  -  One command that assembles the full quarterly review: risk trend, phish-prone trend, training completion, and the top-risk humans.


**repeat-clickers**


- `knowbe4-cli repeat-clickers`  -  Surface users who clicked the bait in two or more distinct phishing security tests within a window  -  your hardest-to-train humans.


**report-rate**


- `knowbe4-cli report-rate`  -  Rank users and groups by how often they report simulated phish versus falling for them - surfacing who never uses the Phish Alert Button.


**risk-drift**


- `knowbe4-cli risk-drift`  -  Rank users and groups by how much their risk score worsened (or improved) between a prior snapshot and now.


**risk-leaderboard**


- `knowbe4-cli risk-leaderboard`  -  Highest-risk users ranked, each enriched with click history, report rate, and overdue-training context.


**untrained-clickers**


- `knowbe4-cli untrained-clickers`  -  Anti-join: users who clicked a phish but have no Passed training enrollment to show for it.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
knowbe4-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Sync then hunt repeat clickers

```bash
knowbe4-cli sync && knowbe4-cli repeat-clickers --min-clicks 2 --since 180d --agent
```

Pull everything local, then surface users who failed two or more phishing tests in the last 180 days.

### Narrow a deeply-nested phishing test payload

```bash
knowbe4-cli phishing-tests get 16142 --agent --select pst_id,name,phish_prone_percentage,clicked_count,reported_count,groups.name
```

PST payloads are large and nested; --select with dotted paths returns only the fields an agent needs.

### Find vulnerable, untrained users

```bash
knowbe4-cli untrained-clickers --since 180d --agent --select user_email,clicked_psts,current_risk_score
```

Anti-join of phishing failures against training records  -  the next enrollment campaign, ready to act on.

### Build a client QBR

```bash
knowbe4-cli qbr --since 90d --format md
```

Assemble risk trend, phish-prone trend, training completion, and top-risk humans into one markdown report.

### Free-text search the synced store

```bash
knowbe4-cli search "finance" --agent
```

Full-text search across synced users, groups, and campaigns without a round-trip to the API.

### Find who never reports phish

```bash
knowbe4-cli report-rate --bottom 25 --agent
```

Rank the users with the worst report-vs-click behavior - the people who never use the Phish Alert Button.

### Check the store before you trust a hunt

```bash
knowbe4-cli freshness --agent
```

Per-table sync freshness plus input-readiness for every cross-entity command - names the sync command that fixes any gap.

## Auth Setup

Authentication uses a Reporting API key (a Bearer token) generated in the KSAT console under Account Settings -> API. Export it as KNOWBE4_API_KEY. If your tenant lives on a non-US server, set KNOWBE4_REGION to eu, ca, uk, or de so the CLI targets the right regional host. The separate User Event API (push custom risk events) uses its own key, KNOWBE4_USER_EVENT_API_KEY, consumed only by the `events` command group.

Run `knowbe4-cli doctor` to verify setup.

## Credential safety

- The Reporting API key (`Authorization: Bearer <KNOWBE4_API_KEY>`) and every Reporting API request go only to the scheme, host and port of the configured base URL (`https://<region>.api.knowbe4.com/v1` built from `KNOWBE4_REGION`, default `us`, or `KNOWBE4_BASE_URL`). A redirect to any other host is refused, so the key never reaches it, and a region that is not a single host-name label is refused before anything is sent.
- The User Event API key (`KNOWBE4_USER_EVENT_API_KEY`, used only by the `events` commands) goes only to the User Event API host for your region (`https://api.events.knowbe4.com`, or `https://api-<region>.events.knowbe4.com`; `KNOWBE4_USER_EVENT_BASE_URL` overrides it), with the same redirect refusal. Neither key is ever sent to the other API.
- Both programs hold every credential they know from the first byte of output: both keys, the MCP HTTP token, and the credentials in `config.toml`, `credentials.toml` and the run profiles. Any output that would carry one of them, in any encoding, is replaced whole by a notice naming where it was going, for example "[credkit: withheld from stdout: it carried a credential this program holds]" (the sink is stdout, stderr, mcp, file, sqlite, outbound request or child process). A request that would carry a held credential to a host it does not belong to, over plain http, or across a redirect to another host is refused before it is sent. Responses are never cached on disk.
- Only `credentials.toml` (mode 0600) holds a credential; `auth set-token` reads the key from stdin, never from an argument.
- `--dry-run` sends nothing. Over MCP, `events_create` and `events_delete` (the only commands that change KnowBe4 data) return a preview and send only with `confirm: true`, and a tool argument can never name a file to read or write.

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
  knowbe4-cli groups list --agent --select id,name,group_type
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Read-only**  -  do not use this CLI for create, update, delete, publish, comment, upvote, invite, order, send, or other mutating requests

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

- Use `--home <dir>` for one invocation, or set `KNOWBE4_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `KNOWBE4_CONFIG_DIR`, `KNOWBE4_DATA_DIR`, `KNOWBE4_STATE_DIR`, `KNOWBE4_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `KNOWBE4_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `knowbe4-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "knowbe4": {
        "command": "knowbe4-mcp",
        "env": {
          "KNOWBE4_HOME": "/srv/knowbe4"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `KNOWBE4_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `KNOWBE4_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are never cached on disk. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `KNOWBE4_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `KNOWBE4_DATA_SOURCE` sets the `--data-source` default.

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
knowbe4-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "knowbe4-cli learnings confirm 12"] }
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
       materially more, record the divergence via `knowbe4-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `knowbe4-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `knowbe4-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
knowbe4-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
knowbe4-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
knowbe4-cli teach-playbook \
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
knowbe4-cli playbook amend \
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

`knowbe4-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `KNOWBE4_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
knowbe4-cli feedback "the --since flag is inclusive but docs say exclusive"
knowbe4-cli feedback --stdin < notes.txt
knowbe4-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `KNOWBE4_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `KNOWBE4_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
knowbe4-cli profile save briefing --json
knowbe4-cli --profile briefing groups list
knowbe4-cli profile list --json
knowbe4-cli profile show briefing
knowbe4-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `knowbe4-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add knowbe4-mcp -- knowbe4-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `knowbe4-mcp --transport http` requires `KNOWBE4_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `KNOWBE4_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `KNOWBE4_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which knowbe4-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   knowbe4-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `knowbe4-cli <command> --help`.
