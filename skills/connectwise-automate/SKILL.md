---
name: connectwise-automate
description: "Every ConnectWise Automate endpoint plus a local SQLite mirror that answers cross-client fleet questions the per-server web UI can't. Trigger phrases: `automate fleet health`, `which automate agents are offline`, `automate patch compliance by client`, `triage automate alerts`, `connectwise automate computers`, `use connectwise-automate`, `run connectwise-automate`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "ConnectWise"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - connectwise-automate-cli
---

# ConnectWise Automate Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `connectwise-automate-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-automate/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-automate/install.ps1 | iex
   ```
3. Verify: `connectwise-automate-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `connectwise-automate-cli` and `connectwise-automate-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

ConnectWise Automate's value is locked behind a per-server console built for one-endpoint-at-a-time work. This CLI syncs your whole fleet  -  computers, clients, locations, alerts, and patch history  -  into local SQLite, then answers the questions MSPs actually ask across clients: where the offline agents are (stale-agents), who's behind on patches (patch-compliance), and what changed overnight (since). Everything is offline, scriptable, and agent-native.

## When to Use This CLI

Use this CLI when an agent or tech needs cross-client answers from ConnectWise Automate  -  fleet-wide health, stale-agent sweeps, patch compliance by client, alert triage, or overnight drift  -  rather than clicking through the per-server console one endpoint at a time. It is also the right tool for scripting Automate reads into other automation and for safe, dry-run-first command/patch actions.

## Unique Capabilities

### Cross-client roll-ups the per-server UI can't do

- **`fleet-health`**  -  See every agent across every client in one roll-up: online/offline, last-contact age, and open-alert count, grouped by client.

  _Reach for this when an agent needs the whole-fleet posture in one shot instead of paging the Computers endpoint client by client._

  ```bash
  connectwise-automate-cli fleet-health --agent
  ```
- **`stale-agents`**  -  List computers not seen in N days, grouped by client  -  the offline agents bleeding license and hiding risk.

  _Use before license true-ups or security reviews to find agents that stopped checking in._

  ```bash
  connectwise-automate-cli stale-agents --days 30 --agent
  ```
- **`patch-compliance`**  -  Per-client patch posture from synced patch history joined to computers  -  worst offenders first.

  _Pull this before a QBR or a security conversation to name the clients that are behind._

  ```bash
  connectwise-automate-cli patch-compliance --agent
  ```
- **`client-rollup`**  -  One-line-per-client snapshot: computers, locations, offline agents, and open alerts  -  built for the client review.

  _The fastest way to brief on a client's whole environment without clicking through the console._

  ```bash
  connectwise-automate-cli client-rollup --agent
  ```

### Triage and inventory

- **`alert-triage`**  -  Open alerts across every client, ranked by priority and joined to computer → location → client, with duplicates collapsed.

  _Start the morning here to see what actually needs a human across the whole book of business._

  ```bash
  connectwise-automate-cli alert-triage --min-priority 3 --agent
  ```
- **`os-inventory`**  -  Fleet-wide operating-system distribution with end-of-life OSes (Windows 7, Server 2008/2012) flagged for upgrade planning.

  _Use for security and hardware-refresh planning when you need the EOL exposure across every client at once._

  ```bash
  connectwise-automate-cli os-inventory --eol-only --agent
  ```
- **`since`**  -  Fleet activity in the last N hours from the records' own timestamps: alerts created, agents that checked in, and patches installed.

  _Run first thing to catch overnight activity across the fleet without paging each endpoint._

  ```bash
  connectwise-automate-cli since --hours 24 --agent
  ```
- **`agent-versions`**  -  Automate agent versions across every client, newest first, with the computers still behind counted per client.

  _Run after a server upgrade to find the agents that did not update: they are the ones that stop patching and running scripts first._

  ```bash
  connectwise-automate-cli agent-versions --behind-only --agent
  ```

`--ca-cert`, `CONNECTWISE_AUTOMATE_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `CONNECTWISE_AUTOMATE_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. `doctor` reports the trust in use and names the setting that fixes a chain or hostname failure.

## Command Reference

**alerts**  -  Open monitor alerts across the fleet

- `connectwise-automate-cli alerts get`  -  Get a single alert by Id
- `connectwise-automate-cli alerts list`  -  List open alerts across all computers

**apitoken**  -  Mint and refresh API bearer tokens

- `connectwise-automate-cli apitoken mint`  -  Mint a bearer token from username + password (needs CONNECTWISE_AUTOMATE_SERVER + clientId header)
- `connectwise-automate-cli apitoken refresh`  -  Refresh an existing (still-valid) bearer token

**clients**  -  Clients (companies / customers) in Automate

- `connectwise-automate-cli clients get`  -  Get a single client by Id
- `connectwise-automate-cli clients list`  -  List all clients

**commands**  -  Available commands that can be executed on agents

- `connectwise-automate-cli commands get`  -  Get a single command by Id
- `connectwise-automate-cli commands list`  -  List all available commands

**computers**  -  Managed endpoints (agents)  -  the core RMM inventory

- `connectwise-automate-cli computers alerts`  -  Open alerts for one computer
- `connectwise-automate-cli computers command-execute`  -  Execute a command on one computer (WRITE  -  runs a real command on the agent)
- `connectwise-automate-cli computers command-history`  -  Recent command execution history for one computer
- `connectwise-automate-cli computers get`  -  Get a single computer by Id
- `connectwise-automate-cli computers list`  -  List computers across the fleet (paginated, filterable)
- `connectwise-automate-cli computers patching-stats`  -  Patch installation statistics for one computer
- `connectwise-automate-cli computers software`  -  Installed software inventory for one computer

**contacts**  -  Client contacts

- `connectwise-automate-cli contacts`  -  List all client contacts

**groups**  -  Computer groups (organizational + policy grouping)

- `connectwise-automate-cli groups get`  -  Get a single group by Id
- `connectwise-automate-cli groups list`  -  List all groups

**locations**  -  Locations (sites) belonging to clients

- `connectwise-automate-cli locations get`  -  Get a single location by Id
- `connectwise-automate-cli locations list`  -  List all locations

**monitors**  -  Monitors and their per-monitor statistics

- `connectwise-automate-cli monitors list`  -  List monitors with their alerting statistics
- `connectwise-automate-cli monitors sensor-checks`  -  List sensor checks

**network-devices**  -  Discovered network devices (non-agent)

- `connectwise-automate-cli network-devices`  -  List discovered network devices

**patching**  -  Patch history, compliance information, and patch policies

- `connectwise-automate-cli patching approval-policies`  -  Patch approval policies
- `connectwise-automate-cli patching deploy-approved`  -  Deploy all approved patches (WRITE  -  triggers fleet patch deployment)
- `connectwise-automate-cli patching deploy-security`  -  Deploy all security patches (WRITE  -  triggers fleet patch deployment)
- `connectwise-automate-cli patching information`  -  Global patch information / catalog status
- `connectwise-automate-cli patching list`  -  Fleet-wide patch installation history
- `connectwise-automate-cli patching microsoft-policies`  -  Microsoft update policies
- `connectwise-automate-cli patching reattempt-failed`  -  Reattempt failed patches (WRITE  -  retries failed patch installs)
- `connectwise-automate-cli patching thirdparty-policies`  -  Third-party update policies

**scripts**  -  Automation scripts and their run state

- `connectwise-automate-cli scripts list`  -  List all scripts
- `connectwise-automate-cli scripts running`  -  List scripts currently running across the fleet
- `connectwise-automate-cli scripts schedules`  -  List scheduled script runs

**server**  -  Automate server metadata (used by doctor / health)

- `connectwise-automate-cli server db-time`  -  Current database server time
- `connectwise-automate-cli server get`  -  Server version and metadata


**agent-versions**


- `connectwise-automate-cli agent-versions`  -  Automate agent versions across every client, newest first, with the computers still behind counted per client.


**alert-triage**


- `connectwise-automate-cli alert-triage`  -  Open alerts across every client, ranked by priority and joined to computer → location → client, with duplicates collapsed.


**client-rollup**


- `connectwise-automate-cli client-rollup`  -  One-line-per-client snapshot: computers, locations, offline agents, and open alerts  -  built for the client review.


**fleet-health**


- `connectwise-automate-cli fleet-health`  -  See every agent across every client in one roll-up: online/offline, last-contact age, and open-alert count, grouped by client.


**os-inventory**


- `connectwise-automate-cli os-inventory`  -  Fleet-wide operating-system distribution with end-of-life OSes (Windows 7, Server 2008/2012) flagged for upgrade planning.


**patch-compliance**


- `connectwise-automate-cli patch-compliance`  -  Per-client patch posture from synced patch history joined to computers  -  worst offenders first.


**since**


- `connectwise-automate-cli since`  -  Fleet activity in the last N hours from the records' own timestamps: alerts created, agents that checked in, and patches installed.


**stale-agents**


- `connectwise-automate-cli stale-agents`  -  List computers not seen in N days, grouped by client  -  the offline agents bleeding license and hiding risk.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
connectwise-automate-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes


### Morning triage across all clients

```bash
connectwise-automate-cli alert-triage --min-priority 3 --agent --select client,computer,priority,message
```

Narrows the alert payload to the four fields that matter so an agent doesn't burn context on the full alert objects.

### Find offline agents for a license true-up

```bash
connectwise-automate-cli stale-agents --days 45 --agent
```

Lists computers not seen in 45 days, grouped by client, straight from the local store.

### Patch posture for a QBR

```bash
connectwise-automate-cli patch-compliance --agent
```

Per-client patch percentages, worst first, ready to paste into a client review.

### Filter live computers without syncing

```bash
connectwise-automate-cli computers list --condition "Status='Offline'" --order-by "LastContact asc" --agent
```

Uses Automate's own condition/orderby query against the live API when you don't want a full sync.

## Auth Setup

Run `connectwise-automate-cli auth setup` for the URL and steps to obtain a token (add `--launch` to open the URL). Then store it:

```bash
echo "$TOKEN" | connectwise-automate-cli auth set-token
```

Or set `CONNECTWISE_AUTOMATE_TOKEN` as an environment variable.

Run `connectwise-automate-cli doctor` to verify setup.

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
  connectwise-automate-cli alerts list --agent --select Id,ComputerId,ComputerName
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

- Use `--home <dir>` for one invocation, or set `CONNECTWISE_AUTOMATE_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `CONNECTWISE_AUTOMATE_CONFIG_DIR`, `CONNECTWISE_AUTOMATE_DATA_DIR`, `CONNECTWISE_AUTOMATE_STATE_DIR`, `CONNECTWISE_AUTOMATE_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `CONNECTWISE_AUTOMATE_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `connectwise-automate-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "connectwise-automate": {
        "command": "connectwise-automate-mcp",
        "env": {
          "CONNECTWISE_AUTOMATE_HOME": "/srv/connectwise-automate"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `CONNECTWISE_AUTOMATE_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `CONNECTWISE_AUTOMATE_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are cached for five minutes under the cache dir. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `CONNECTWISE_AUTOMATE_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `CONNECTWISE_AUTOMATE_DATA_SOURCE` sets the `--data-source` default.

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
connectwise-automate-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "connectwise-automate-cli learnings confirm 12"] }
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
       materially more, record the divergence via `connectwise-automate-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `connectwise-automate-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `connectwise-automate-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
connectwise-automate-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
connectwise-automate-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
connectwise-automate-cli teach-playbook \
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
connectwise-automate-cli playbook amend \
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

`connectwise-automate-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `CONNECTWISE_AUTOMATE_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
connectwise-automate-cli feedback "the --since flag is inclusive but docs say exclusive"
connectwise-automate-cli feedback --stdin < notes.txt
connectwise-automate-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `CONNECTWISE_AUTOMATE_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `CONNECTWISE_AUTOMATE_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
connectwise-automate-cli profile save briefing --json
connectwise-automate-cli --profile briefing alerts list
connectwise-automate-cli profile list --json
connectwise-automate-cli profile show briefing
connectwise-automate-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `connectwise-automate-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up), then register it:

```bash
claude mcp add connectwise-automate-mcp -- connectwise-automate-mcp
```

Verify: `claude mcp list`

For a remote host, `connectwise-automate-mcp --transport http` requires `CONNECTWISE_AUTOMATE_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `CONNECTWISE_AUTOMATE_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `CONNECTWISE_AUTOMATE_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which connectwise-automate-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   connectwise-automate-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `connectwise-automate-cli <command> --help`.
