---
name: cisco-meraki
description: "Use when the user asks about Cisco Meraki across one or many client organizations: what is offline or alerting right now, failed MX WAN uplinks, license, subscription and end-of-support renewals, who changed the Dashboard config, which org owns a MAC, IP or serial, admins without two-factor or a departed tech's access, or any Meraki Dashboard API operation. Trigger phrases: `which meraki devices are offline`, `meraki license expirations across clients`, `find this mac in meraki`, `who changed the meraki config`, `did a change take this site down`, `meraki admins without 2fa`, `use cisco-meraki`, `run cisco-meraki`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Cisco Meraki"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - cisco-meraki-cli
    install:
      - kind: script
        bins: [cisco-meraki-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-meraki/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-meraki/install.ps1
---

# Cisco Meraki - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `cisco-meraki-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-meraki/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-meraki/install.ps1 | iex
   ```
3. Verify: `cisco-meraki-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `cisco-meraki-cli` and `cisco-meraki-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

One API key reaches every organization you administer, so this CLI treats them as one fleet: fleet health and fleet down show what is broken across all clients, fleet renewals lists license and hardware deadlines, and fleet find resolves a MAC, IP or serial to the client that owns it. Fleet commands walk every Link-header page; when some orgs fail they still exit 0 with the rows they could read and name the failed orgs on stderr (fleet health also marks each unreachable org's row, and fleet fanout carries the error in each org's row under --json). Generated list commands read every page with --all, and every write supports --dry-run.

## When to Use This CLI

Use this CLI when an agent needs to read or change Cisco Meraki Dashboard state, especially across many client organizations at once: fleet health, down devices, license and EoX deadlines, change forensics, admin audits, and locating a device or client by MAC, IP or serial. Most fleet commands read live; fleet changes and fleet find can answer from the local mirror after a sync. History is limited by the API: 31 days for fleet flaps and fleet blame, 365 days for fleet changes.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI for Meraki Go devices; they are managed by a separate consumer app and API
- Do not use it for Catalyst Center, ISE or IOS XE device CLI configuration
- Do not use it to watch for real-time events; configure Dashboard webhooks instead
- Do not use it for UniFi, Fortinet or other vendors' gear

## Unique Capabilities

### Every client org at once
- **`fleet health`**  -  One worst-first row per client org with online, alerting, offline and dormant device counts, failed WAN uplinks, active alerts, and whether the org is reachable at all.

  _Reach for this first when asked which client is in the worst shape right now; it answers in one call instead of one call per org._

  ```bash
  cisco-meraki-cli fleet health --agent
  ```
- **`fleet down`**  -  Every offline or alerting device and every failed MX WAN uplink across all orgs, with org and network names and, for offline devices, how long since the device last reported.

  _Use this to build a dispatch list of individual down devices and sites running on backup WAN across every client._

  ```bash
  cisco-meraki-cli fleet down --agent
  ```
- **`fleet find`**  -  Paste a MAC, IP, serial, network ID or name and get the org and network that own the matching device, inventory entry, MX uplink or, by MAC, client.

  _Use this whenever a ticket or alert names a device or client without saying which client org it belongs to._

  ```bash
  cisco-meraki-cli fleet find Q2QN-9J8L-SLPD --agent
  ```
- **`fleet fanout`**  -  Run any organization-scoped read against every org, fully paginated, with observed-at, item count, completeness and next-cursor metadata per org (use `--json`: `--agent` compacts away `next_cursor`, `limit_reason` and `error`).

  _Use this for any cross-org read that has no dedicated command, when you need to know the answer is complete._

  ```bash
  cisco-meraki-cli fleet fanout organizations/{organizationId}/firmware/upgrades --json
  ```

### Deadlines and access audits
- **`fleet renewals`**  -  License expirations (co-term, per-device or subscription, whichever each org uses) and end-of-sale and end-of-support hardware dates in one date-sorted list.

  _Use this for renewal and hardware-refresh pipelines across every client org._

  ```bash
  cisco-meraki-cli fleet renewals --within 90d --agent
  ```
- **`fleet admins`**  -  Dashboard admins across every org grouped by email, flagging missing two-factor auth, API key holders and stale access, with an offboarding view per person.

  _Use this for access reviews and technician offboarding across all client orgs._

  ```bash
  cisco-meraki-cli fleet admins --stale 90d --agent
  ```

### Change and outage history
- **`fleet changes`**  -  Configuration changes from every org in one list, filterable by admin, org, network and page, with old and new values.

  _Use this to answer who changed what in which client org this week._

  ```bash
  cisco-meraki-cli fleet changes --since 7d --agent
  ```
- **`fleet flaps`**  -  Devices ranked by minutes offline, then by number of status changes, over a window of up to 31 days, across every org.

  _Use this for the weekly look back at unstable devices, not for what is down right now._

  ```bash
  cisco-meraki-cli fleet flaps --since 7d --agent
  ```
- **`fleet blame`**  -  For one network ID or device serial: each time it went offline or started alerting within --since (default 7 days, up to 31), the configuration changes made on that network just before.

  _Use this in outage postmortems to find the change that probably caused it._

  ```bash
  cisco-meraki-cli fleet blame L_646829496481105433 --window 2h --agent
  ```

### Safe writes at fleet scale
- **`fleet push`**  -  Apply one settings change to many networks across orgs through action batches: it previews a per-network current-versus-new diff by default and sends batches only with --apply.

  _Use this for bulk configuration changes across many client networks; read the preview, then rerun with --apply._

  ```bash
  cisco-meraki-cli fleet push --path /networks/{networkId}/snmp --body snmp.json --network-tag managed
  ```

## Command Reference

The list below is a summary. The full tree has 1035 typed operations under `organizations`, `networks`, `devices` and `administered`; before concluding a capability is missing, run `cisco-meraki-cli which "offline devices" --json` or a group's help, e.g. `cisco-meraki-cli organizations --help`.

**administered**  -  Manage administered

- `cisco-meraki-cli administered bind-licensing-subscription-subscription`  -  Bind networks to a subscription
- `cisco-meraki-cli administered claim-licensing-subscription-subscriptions`  -  Claim a subscription into an organization.
- `cisco-meraki-cli administered generate-identities-me-api-keys`  -  Generates an API key for an identity.
- `cisco-meraki-cli administered get-identities-me`  -  Returns the identity of the current user.
- `cisco-meraki-cli administered get-identities-me-api-keys`  -  List the non-sensitive metadata associated with the API keys that belong to the user
- `cisco-meraki-cli administered get-licensing-subscription-entitlements`  -  Retrieve the list of purchasable entitlements
- `cisco-meraki-cli administered get-licensing-subscription-subscriptions`  -  List available subscriptions
- `cisco-meraki-cli administered get-licensing-subscription-subscriptions-compliance-statuses`  -  Get compliance status for requested subscriptions
- `cisco-meraki-cli administered revoke-identities-me-api-keys`  -  Revokes an identity's API key, using the last four characters of the key.
- `cisco-meraki-cli administered validate-licensing-subscription-subscriptions-claim-key`  -  Find a subscription by claim key. Returns 400 if the key has already been claimed.

**devices**  -  Manage devices

- `cisco-meraki-cli devices get`  -  Return a single device
- `cisco-meraki-cli devices update`  -  Update the attributes of a device

**networks**  -  Manage networks

- `cisco-meraki-cli networks delete`  -  Delete a network
- `cisco-meraki-cli networks get`  -  Return a network
- `cisco-meraki-cli networks update`  -  Update a network

**organizations**  -  Manage organizations

- `cisco-meraki-cli organizations create`  -  Create a new organization
- `cisco-meraki-cli organizations delete`  -  Delete an organization
- `cisco-meraki-cli organizations get`  -  List the organizations that the user has privileges on
- `cisco-meraki-cli organizations get-organizationid`  -  Return an organization
- `cisco-meraki-cli organizations update`  -  Update an organization


**fleet**


- `cisco-meraki-cli fleet health`  -  One worst-first row per client org with online, alerting, offline and dormant device counts, failed WAN uplinks, active alerts, and whether the org is reachable at all.

- `cisco-meraki-cli fleet down`  -  Every offline or alerting device and every failed MX WAN uplink across all orgs, with org and network names and, for offline devices, how long since the device last reported.

- `cisco-meraki-cli fleet renewals`  -  License expirations (co-term, per-device or subscription, whichever each org uses) and end-of-sale and end-of-support hardware dates in one date-sorted list.

- `cisco-meraki-cli fleet changes`  -  Configuration changes from every org in one list, filterable by admin, org, network and page, with old and new values.

- `cisco-meraki-cli fleet find`  -  Paste a MAC, IP, serial, network ID or name and get the org and network that own the matching device, inventory entry, MX uplink or, by MAC, client.

- `cisco-meraki-cli fleet fanout`  -  Run any organization-scoped read against every org, fully paginated, with observed-at, item count, completeness and next-cursor metadata per org.

- `cisco-meraki-cli fleet push`  -  Apply one settings change to many networks across orgs through action batches: it previews a per-network current-versus-new diff by default and sends batches only with --apply.

- `cisco-meraki-cli fleet admins`  -  Dashboard admins across every org grouped by email, flagging missing two-factor auth, API key holders and stale access, with an offboarding view per person.

- `cisco-meraki-cli fleet flaps`  -  Devices ranked by minutes offline, then by number of status changes, over a window of up to 31 days, across every org.

- `cisco-meraki-cli fleet blame`  -  For one network ID or device serial: each time it went offline or started alerting within --since (default 7 days, up to 31), the configuration changes made on that network just before.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
cisco-meraki-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Morning fleet triage

```bash
cisco-meraki-cli fleet down --agent --select organization_name,network_name,serial,kind,status,down_for
```

Narrow the cross-org down list to the fields a dispatcher needs.

### Who touched which client this week

```bash
cisco-meraki-cli fleet changes --since 7d --group-by admin --agent
```

Group every org's configuration changes by admin for the weekly review.

### Locate a MAC from a SOC alert

```bash
cisco-meraki-cli fleet find 00:18:0a:12:34:56 --agent
```

Resolve a client MAC to the org and network that owns it without guessing.

### Renewal pipeline

```bash
cisco-meraki-cli fleet renewals --within 90d --csv
```

Export license and end-of-support deadlines due within 90 days, plus overdue ones, across all orgs for quoting; add --include-expired=false to hide overdue items (CLI only: an MCP call cannot switch this default-on flag off, so filter out rows with negative days_remaining instead).

### Preview a bulk SNMP change

```bash
cisco-meraki-cli fleet push --path /networks/{networkId}/snmp --body snmp.json --network-tag managed
```

Shows which networks would change and which already match; nothing is sent until you rerun the same command with --apply.

## Auth Setup

Meraki uses one Dashboard API key, sent as the X-Cisco-Meraki-API-Key header. Enable API access for each organization (Organization > Settings > Dashboard API access), then generate a key under My Profile > API access. The key reaches every organization that admin can see, which is what the fleet commands fan out across. Export it as MERAKI_DASHBOARD_API_KEY or store it with auth set-token. Dashboards in Canada, China, India or FedRAMP need CISCO_MERAKI_BASE_URL set to that region's host.

Run `cisco-meraki-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, sync, and `--deliver` paths:

- `--json`  -  one JSON document on stdout (sync progress events go to stderr)
- `--compact`  -  keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain`  -  tabular rows (collection envelopes unwrap to the row array)
- `--quiet`  -  one identity value per row, no envelope

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  cisco-meraki-cli devices get Q2QN-9J8L-SLPD --agent --select serial,name,model,networkId,lanIp
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **No confirmation prompts**  -  writes and deletes run as soon as they are invoked, with no confirmation prompt; preview with `--dry-run` (or run `fleet push` without `--apply`) before mutating. `--yes` is only read by `profile delete` and client-profile commands.
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success, and use `--ignore-missing` only when a missing delete target should count as success

### Response envelope

Generated commands that read from the local store or the API wrap output in a provenance envelope (fleet commands add it only under `--agent`; with `--json` they print a bare array):

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set  -  piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Paths and state

Agents should treat the CLI's path resolver as part of the runtime contract:

- Use `--home <dir>` for one invocation, or set `CISCO_MERAKI_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `CISCO_MERAKI_CONFIG_DIR`, `CISCO_MERAKI_DATA_DIR`, `CISCO_MERAKI_STATE_DIR`, `CISCO_MERAKI_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `CISCO_MERAKI_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, the local mirror (`data-<hash>.db`, one per API key), cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `cisco-meraki-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "cisco-meraki": {
        "command": "cisco-meraki-mcp",
        "env": {
          "CISCO_MERAKI_HOME": "/srv/cisco-meraki"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `CISCO_MERAKI_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `CISCO_MERAKI_HOME`, or `doctor` will not find credentials left under the former root.

## Automatic learning

This CLI ships a self-capturing learning loop. The CLI does its own bookkeeping: every invocation is journaled locally, a failed flag followed by a corrected retry auto-derives a `flag_alias` candidate, and a `teach` on a query family without a playbook auto-synthesizes a `playbook_candidate` from the session's journal. Your job is judgment only: `recall` first, act on surfaced candidates, `teach` the final answer, `playbook amend` when you observe a correction. You never record failures by hand.

### Step 1: `recall` before any discovery

Before list/search/drill commands on a new user question, pass the question as an argv or MCP tool argument to `recall --json` (under `--agent` the envelope below arrives wrapped as `{"meta": ..., "results": <envelope>}`). Do not interpolate user-controlled text into a shell command line.

Quoted `recall "<question>"` breaks on an apostrophe, which is ordinary English. A quoted heredoc breaks when a body line equals the delimiter, and that delimiter is published in these docs. Write the question with a non-shell file-writing tool, then read it back as data:

```bash
# Write the question verbatim with your file-writing tool (no shell involved).
# Command substitution on a file only ever yields data  -  the shell never
# parses the file's bytes as syntax.
QUERY=$(cat /path/to/question.txt)
cisco-meraki-cli recall "$QUERY" --json
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
      "next_action": ["<trial command>", "cisco-meraki-cli learnings confirm 12"] }
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
       materially more, record the divergence via `cisco-meraki-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `cisco-meraki-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `cisco-meraki-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
cisco-meraki-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
cisco-meraki-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
cisco-meraki-cli teach-playbook \
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
cisco-meraki-cli playbook amend \
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

`cisco-meraki-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `CISCO_MERAKI_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
cisco-meraki-cli feedback "the --since flag is inclusive but docs say exclusive"
cisco-meraki-cli feedback --stdin < notes.txt
cisco-meraki-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `CISCO_MERAKI_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `CISCO_MERAKI_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
cisco-meraki-cli profile save briefing --json
cisco-meraki-cli --profile briefing devices get Q2QN-9J8L-SLPD
cisco-meraki-cli profile list --json
cisco-meraki-cli profile show briefing
cisco-meraki-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Generic error |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 6 | Partial failure |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `cisco-meraki-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add cisco-meraki-mcp --env MERAKI_DASHBOARD_API_KEY=<your-api-key> -- cisco-meraki-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which cisco-meraki-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   cisco-meraki-cli fleet down --agent
   ```
4. If ambiguous, drill into subcommand help, e.g. `cisco-meraki-cli organizations devices --help`.
