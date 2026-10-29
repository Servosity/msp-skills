---
name: liongard
description: "Every Liongard endpoint, plus an offline copy of your whole MSP estate you can join, search, and drift-check from one command. Trigger phrases: `what changed across my Liongard environments`, `show stale Liongard launchpoints`, `which Liongard agents are offline`, `pivot a Liongard metric across all systems`, `Liongard drift report`, `use liongard`, `run liongard`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Liongard"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - liongard-cli
    install:
      - kind: script
        bins: [liongard-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/liongard/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/liongard/install.ps1
---

# Liongard - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `liongard-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/liongard/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/liongard/install.ps1 | iex
   ```
3. Verify: `liongard-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `liongard-cli` and `liongard-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Liongard's API answers one environment at a time and only through the web UI or per-instance REST calls. This CLI syncs every environment, system, launchpoint, inspector, agent, detection, metric, and timeline entry into a local SQLite store, then runs cross-estate joins the live API never returns: drift since a window, stale launchpoints, offline agents, monitoring-coverage gaps, and metric pivots. Agent-native throughout  -  --json, --select, --csv, and typed exit codes.

## When to Use This CLI

Reach for this CLI when an agent needs whole-estate Liongard answers  -  what changed across all clients, which collectors are stale, which agents are offline, where monitoring coverage is missing, or one metric pivoted across every system. It is the right tool for scripted MSP health sweeps and QBR/SLA reporting where the per-environment web UI and one-call-at-a-time API are too slow. Prefer it over raw API calls whenever the question spans more than one environment or needs offline, structured, composable output.

## Unique Capabilities

### Whole-estate visibility
- **`drift`**  -  Every change detected across all your client environments within a time window, joined to the owning environment and system.

  _Reach for this when an agent needs the whole-estate 'what changed overnight' answer instead of paging the per-environment detections endpoint._

  ```bash
  liongard-cli drift --since 24h --agent
  ```
- **`environments overview`**  -  One client's complete picture in a single command: its systems, each system's launchpoints, agent, latest inspection, open detections, and key metrics.

  _Use when an agent is asked about a specific client and needs the joined picture rather than five separate endpoint calls._

  ```bash
  liongard-cli environments overview 42 --full --agent
  ```
- **`systems history`**  -  The full chronological change history of one system: every detection and inspection entry in time order.

  _Reach for this when an agent needs 'when did this system change / break' instead of paging timeline and detections endpoints separately._

  ```bash
  liongard-cli systems history 4821 --agent
  ```

### Inspection health
- **`launchpoints stale`**  -  Every launchpoint whose newest inspection is older than a threshold, with the owning environment and system named.

  _Use to find collectors that silently stopped reporting before a client notices their data is old._

  ```bash
  liongard-cli launchpoints stale --older-than 7d --agent
  ```
- **`agents offline`**  -  Every offline agent across the estate, joined to the environment it serves.

  _Use for the daily 'is everything still collecting' health sweep across all clients at once._

  ```bash
  liongard-cli agents offline --agent
  ```
- **`coverage`**  -  Monitoring gaps: systems with no launchpoint bound, and environments with no systems at all.

  _Use during onboarding QA to catch clients you provisioned but never pointed an inspector at._

  ```bash
  liongard-cli coverage --agent
  ```
- **`launchpoints run-stale`**  -  Find stale launchpoints and trigger an inspection run on each, in one guarded command.

  _Use to re-kick every collector that fell behind, instead of clicking run on each launchpoint in the UI._

  ```bash
  liongard-cli launchpoints run-stale --older-than 7d
  ```
- **`detections failures`**  -  Every inspection that ran but failed or errored across the estate, joined to the owning environment.

  _Reach for this to find inspections that ran but errored - a different failure mode from collectors that silently stopped reporting (launchpoints stale)._

  ```bash
  liongard-cli detections failures --since 7d --agent
  ```
- **`inspectors coverage`**  -  Which environments are missing a given inspector type - the estate-wide rollout-gap view.

  _Reach for this when an agent needs 'who is missing the M365 inspector' instead of paging launchpoints per environment._

  ```bash
  liongard-cli inspectors coverage --inspector "Microsoft 365" --agent
  ```
- **`health`**  -  One estate-wide health scorecard: stale launchpoints, offline agents, failed inspections, and coverage gaps as a single summary with a typed exit code.

  _Reach for this for a single-command daily health check with a typed exit code, instead of running each sweep separately._

  ```bash
  liongard-cli health --agent
  ```

### Reporting and SLA
- **`metrics pivot`**  -  One RoarPath metric across every system it covers, CSV-ready for reports; the value column is filled only where the local store carries an evaluated value.

  _Use to assemble a single metric across the whole estate for an SLA or QBR deck in one command._

  ```bash
  liongard-cli metrics pivot "MFA Enabled Count" --csv
  ```
- **`metrics breach`**  -  Every system whose RoarPath metric value crosses a numeric threshold. It refuses until every covered system has an evaluated value in the local store; the synced metric definitions carry none (Liongard evaluates them per system: `liongard-cli metrics evaluation`).

  _Use for SLA-breach and security-posture checks across all clients without N per-system calls._

  ```bash
  liongard-cli metrics breach "Patch Age Days" --op gt --value 30 --agent
  ```

## Command Reference

**access-keys**  -  Manage access keys

- `liongard-cli access-keys create`  -  Create an Access Token with the permission of user or with only 'Add Agent' permission
- `liongard-cli access-keys delete`  -  Delete Access Token created by user
- `liongard-cli access-keys get`  -  Returns a List of Access Tokens created by user
- `liongard-cli access-keys get-count`  -  Return a count of all Access Tokens created by user

**agents**  -  Agents can be installed in the Cloud or On-Premise and are responsible for running inspections.

- `liongard-cli agents delete`  -  Remove an agent
- `liongard-cli agents get`  -  List all agents.
- `liongard-cli agents get-agentid`  -  Get a specific Agent.
- `liongard-cli agents get-count`  -  Returns a count of all the agents in your service provider.
- `liongard-cli agents update`  -  Edits a deployed liongard agent, cannot update On-Demand agents.

**authentication**  -  Manage authentication

- `liongard-cli authentication log-in`  -  Returns a session token as well as additional information about logged-in user
- `liongard-cli authentication verify-token`  -  Verify token with MFA authentication code

**detections**  -  A Detection is a Change that was detected on a system.

- `liongard-cli detections detections`  -  Returns a list of all detection events.
- `liongard-cli detections detectionsby-id`  -  Gets a specific detection.
- `liongard-cli detections get-count1234`  -  Returns count of all detection events.

**environments**  -  An environment in Roar represents a single end customer for your MSP. An environment will contain Agents, Launchpoints (configured inspections), and Systems (data that has landed as a result of an inspection).

- `liongard-cli environments count`  -  Returns a count of all environments in your Service Provider.
- `liongard-cli environments create`  -  Creates a single Liongard Environment.
- `liongard-cli environments create-bulk`  -  Create many environments at once.
- `liongard-cli environments delete`  -  Delete an environment.
- `liongard-cli environments get`  -  Fetch all environments in your Service Provider.
- `liongard-cli environments get-environmentid`  -  Get a single environment.
- `liongard-cli environments update`  -  Update a single environment, If you want to update a child environment, set the Parent Environment, if not set to null

**groups**  -  A Group represents a combined set of permissions that can be assigned to Users

- `liongard-cli groups`  -  Returns a List of available Assignable Roles for a user

**inspector**  -  An Inspector represents the system type and used for building the config templates for launchpoints


**inspectors**  -  An Inspector represents the system type and used for building the config templates for launchpoints

- `liongard-cli inspectors`  -  Lists all avaialble Inspectors in Liongard

**launchpoints**  -  A Launchpoint is a configured Inspection. As an example, if you set up a domain inspector to run on liongard.com, that would be considered a Launchpoint.

- `liongard-cli launchpoints bulk-delete`  -  Remove all launchpoints.
- `liongard-cli launchpoints bulk-run`  -  Kick off many inspections.
- `liongard-cli launchpoints delete`  -  Remove a single launchpoint.
- `liongard-cli launchpoints get`  -  Lists all launchpoints.
- `liongard-cli launchpoints get-count`  -  Returns a count of all launchpoints.
- `liongard-cli launchpoints launchpoint`  -  Create a launchpoint.
- `liongard-cli launchpoints launchpointsby-id`  -  Return a specific launchpoint by ID.
- `liongard-cli launchpoints update`  -  Edit a single inspector launchpoint.
- `liongard-cli launchpoints update-bulk`  -  Update many launchpoints to run on the same schedule.

**logs**  -  Manage logs

- `liongard-cli logs`  -  Return the logs for a specific inspection.

**metrics**  -  Manage metrics

- `liongard-cli metrics create`  -  Creates a Single Metrics for a system requires valid JMESPath in the Query field
- `liongard-cli metrics delete`  -  Deletes only Custom Created Metrics, Liongard Created Metrics can not be deleted
- `liongard-cli metrics evaluation`  -  For each system ID passed
- `liongard-cli metrics evaluation-post`  -  For each system ID passed
- `liongard-cli metrics metrics`  -  Returns a list of metrics that have been created.
- `liongard-cli metrics update`  -  Updates a Metric requires valid JMESPath in the Query field

**systems**  -  A System in Roar represents a system that has been inspected by a Launchpoint. When the inspection completes for the first time, a system is completed and a corresponding timeline entry is created each time the inspection lands.

- `liongard-cli systems get-count`  -  Count of all systems in your service provider.
- `liongard-cli systems systems`  -  List all systems.

**tasks**  -  Manage tasks

- `liongard-cli tasks get-alert`  -  Returns a single alert that has been raised.
- `liongard-cli tasks get-count12345`  -  Returns a count of all alerts.
- `liongard-cli tasks list-alerts`  -  Returns a list of alerts that have been raised.

**timeline**  -  A Timeline entry represents a single inspection event. It links a System to it's configuration at that point in time.

- `liongard-cli timeline get-count123`  -  Return count of all timeline entries.
- `liongard-cli timeline timeline`  -  Fetch all timeline entries.
- `liongard-cli timeline timelineby-id`  -  Fetch a specific timeline.

**users**  -  Create and manage users in your Liongard instance

- `liongard-cli users count`  -  Returns a count of total users
- `liongard-cli users create`  -  When creating and using the Manger or Reader Roles it is required to use either the Environment/EnvironmentID or
- `liongard-cli users delete`  -  Remove a single User.
- `liongard-cli users get`  -  Returns a list of users in your Liongard Instance
- `liongard-cli users get-single`  -  Returns a Single User
- `liongard-cli users update`  -  Updates a single User


**agents**


- `liongard-cli agents offline`  -  Every offline agent across the estate, joined to the environment it serves.


**coverage**


- `liongard-cli coverage`  -  Monitoring gaps: systems with no launchpoint bound, and environments with no systems at all.


**detections**


- `liongard-cli detections failures`  -  Every inspection that ran but failed or errored across the estate, joined to the owning environment.


**drift**


- `liongard-cli drift`  -  Every change detected across all your client environments within a time window, joined to the owning environment and system.


**environments**


- `liongard-cli environments overview`  -  One client's complete picture in a single command: its systems, each system's launchpoints, agent, latest inspection, open detections, and key metrics.


**health**


- `liongard-cli health`  -  One estate-wide health scorecard: stale launchpoints, offline agents, failed inspections, and coverage gaps as a single summary with a typed exit code.


**inspectors**


- `liongard-cli inspectors coverage`  -  Which environments are missing a given inspector type - the estate-wide rollout-gap view.


**launchpoints**


- `liongard-cli launchpoints stale`  -  Every launchpoint whose newest inspection is older than a threshold, with the owning environment and system named.

- `liongard-cli launchpoints run-stale`  -  Find stale launchpoints and trigger an inspection run on each, in one guarded command.


**metrics**


- `liongard-cli metrics pivot`  -  One RoarPath metric across every system it covers, CSV-ready for reports; the value column is filled only where the local store carries an evaluated value.

- `liongard-cli metrics breach`  -  Every system whose RoarPath metric value crosses a numeric threshold. It refuses until every covered system has an evaluated value in the local store; the synced metric definitions carry none (Liongard evaluates them per system: `liongard-cli metrics evaluation`).


**systems**


- `liongard-cli systems history`  -  The full chronological change history of one system: every detection and inspection entry in time order.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
liongard-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Morning drift triage as JSON

```bash
liongard-cli drift --since 24h --agent --select environment,system,name
```

Whole-estate change feed for the last day, projected to just the fields an agent needs.

### Estate-wide stale collector report

```bash
liongard-cli launchpoints stale --older-than 7d --csv
```

CSV of every launchpoint that stopped reporting, ready to paste into a ticket.

### Pivot one metric across all systems

```bash
liongard-cli metrics pivot "MFA Enabled Count" --csv
```

A system-by-value table for a single RoarPath metric across the estate.

### Narrow a verbose detections payload

```bash
liongard-cli detections detections --agent --select ID,EnvironmentID,Name
```

Detections responses are large; --select keeps only the high-gravity fields so an agent does not burn context.

### Find security-posture breaches

```bash
liongard-cli metrics breach "Patch Age Days" --op gt --value 30 --agent
```

Every system whose patch age crosses the threshold, across all clients.

## Auth Setup

Liongard issues an Access Key ID and an Access Key Secret per user. The CLI sends them as the X-ROAR-API-KEY header (base64 of `accessKeyId:accessKeySecret`). Set LIONGARD_INSTANCE (your subdomain, e.g. us1), LIONGARD_ACCESS_KEY_ID, and LIONGARD_ACCESS_KEY_SECRET; or set a pre-encoded LIONGARD_API_KEY directly. Run `doctor` to confirm the host and credentials resolve.

Run `liongard-cli doctor` to verify setup.

## Credential safety

- The `X-ROAR-API-KEY` header (and the `X-Auth-Token` session token) and every API request go only to the scheme, host and port of the resolved base URL (`https://<LIONGARD_INSTANCE>.app.liongard.com/api/v1`, or `LIONGARD_BASE_URL`). A redirect to any other host is refused, so the credential never reaches it.
- Any output, file or MCP result that would repeat `LIONGARD_API_KEY`, the Access Key ID or Secret, the header composed from them, the session token, or a credential-named value from a response (a new access key's secret, for example) prints "[text withheld: it repeated a secret value this program holds]" in its place. Responses are never cached on disk, and credential-named fields are dropped before anything reaches the local store.
- Only `credentials.toml` (mode 0600) holds a credential; `auth set-token` reads the token from stdin, never from an argument.
- `--dry-run` sends nothing. Over MCP, a tool that creates, changes, deletes or runs something in Liongard (`liongard_execute`, `launchpoints run-stale`, `import`) returns a preview and sends only with `confirm: true`, and a tool argument can never name a file to read or write.

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
  liongard-cli access-keys get --agent --select AccessKeyId,CreatedOn,DeletedOn
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

- Use `--home <dir>` for one invocation, or set `LIONGARD_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `LIONGARD_CONFIG_DIR`, `LIONGARD_DATA_DIR`, `LIONGARD_STATE_DIR`, `LIONGARD_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `LIONGARD_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `liongard-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "liongard": {
        "command": "liongard-mcp",
        "env": {
          "LIONGARD_HOME": "/srv/liongard"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `LIONGARD_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `LIONGARD_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are never cached on disk. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `LIONGARD_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `LIONGARD_DATA_SOURCE` sets the `--data-source` default.

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
liongard-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "liongard-cli learnings confirm 12"] }
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
       materially more, record the divergence via `liongard-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `liongard-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `liongard-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
liongard-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
liongard-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
liongard-cli teach-playbook \
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
liongard-cli playbook amend \
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

`liongard-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `LIONGARD_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
liongard-cli feedback "the --since flag is inclusive but docs say exclusive"
liongard-cli feedback --stdin < notes.txt
liongard-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `LIONGARD_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `LIONGARD_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
liongard-cli profile save briefing --json
liongard-cli --profile briefing access-keys get
liongard-cli profile list --json
liongard-cli profile show briefing
liongard-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `liongard-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add liongard-mcp -- liongard-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `liongard-mcp --transport http` requires `LIONGARD_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `LIONGARD_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `LIONGARD_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which liongard-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   liongard-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `liongard-cli <command> --help`.
