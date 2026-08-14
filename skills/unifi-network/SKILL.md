---
name: unifi-network
description: "Use when the user asks to audit a UniFi Network gateway: list adopted devices and connected clients, review firewall policies, ACL rules, VLANs and WiFi broadcasts, see what changed in site config since the last check, spot hardware that just joined the network, or check switch port and PoE headroom. Wraps the local UniFi Network integration API on a self-hosted UniFi OS gateway - not the Site Manager cloud API, UniFi Protect, or UniFi Access - plus a local SQLite mirror that answers change-over-time questions the live API cannot. Trigger phrases: `audit my unifi network`, `what changed on my unifi site`, `list unifi devices`, `which clients are on my unifi network`, `check unifi firewall rules`, `unifi port audit`, `who just joined the network`, `use unifi-network-cli`, `run unifi-network-cli`."
author: "Ricardo Cabral"
license: "Apache-2.0"
vendor: "UniFi"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - unifi-network-cli
---

# UniFi Network - Claude Code Skill

> Unofficial. Community-built Claude Code Skill and MCP server for the UniFi
> Network integration API. Not affiliated with, endorsed by, or sponsored by
> Ubiquiti Inc.

## Prerequisites: Install the CLI

This skill drives the `unifi-network-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/unifi-network/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/unifi-network/install.ps1 | iex
   ```
3. Verify: `unifi-network-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed with skill commands until verification succeeds.

unifi-network-cli wraps the full local Network integration API (devices, clients, firewall, ACL, networks, VPN, switching) with a local SQLite mirror. That mirror is what lets it answer questions the live API can't: what changed since yesterday, what device just joined, and which firewall rule would match a given packet.

## When to Use This CLI

Use this CLI for scripted or agent-driven UniFi Network audits, firewall/ACL review, and change-detection on a self-hosted gateway. Built for MSP owners and technicians who want terminal-first, agent-drivable control over a client's UniFi controller instead of clicking the web UI.

## Anti-triggers

Do not use this CLI for:
- Do not use this for UniFi Protect (cameras) or UniFi Access (doors)  -  this CLI only covers the Network integration API.
- Do not use this for the legacy cloud/CloudKey Controller API  -  that's a different auth model entirely.
- Do not treat rule-predict's output as a live-gateway guarantee  -  it simulates against the last synced ruleset.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Local state that compounds
- **`topology`**  -  Group every synced client under the device it is attached to, a device-to-client tree built entirely from local mirror data, no live crawl needed.

  _Reach for this when an agent needs to see which clients sit behind which device without walking every device endpoint individually._

  ```bash
  unifi-network-cli topology --site default --json
  ```
- **`drift`**  -  Show what changed in site config (networks, firewall, wifi, DNS) since the last drift run for this site.

  _Use after a suspected config change to see exactly what moved, without manually diffing the controller UI._

  ```bash
  unifi-network-cli drift --site default --since 24h --json
  ```

  `--since` is echoed in the output for reference only - the comparison is always
  against the snapshot the previous `drift` run captured, not a fixed lookback
  window. The first run for a site captures the baseline and reports no changes.
- **`newcomer`**  -  List devices and clients first seen since a given sync, for spotting new hardware joining the network.

  _Use for periodic security review of what joined the network recently._

  ```bash
  unifi-network-cli newcomer --since 7d --json
  ```

### Agent-native plumbing
- **`port-audit`**  -  Review port utilization and PoE status across every switch on a site. Pass `--json` for per-port detail; the plain terminal path prints one `N up / M down, PoE active on K port(s)` line per device. It reads the device list from the local mirror but fetches interfaces live, one call per switching or gateway device.

  _Use before adding new PoE devices to check headroom, or to find unused ports across a stack._

  ```bash
  unifi-network-cli port-audit --site default --json
  ```
- **`guest report`**  -  Summarize guest network usage: active vouchers and connected guest clients, from local data.

  _Use for a quick guest-network health check without cross-referencing separate voucher and client screens. Note the output contains live voucher codes._

  ```bash
  unifi-network-cli guest report --site default --json
  ```
- **`rule-predict`**  -  Predict which firewall policy would match a hypothetical packet before making a live change. Matches on source and destination IP only: `--port` is echoed for reference and is not used for matching. **Pass host IPs, not CIDRs**  -  a CIDR's mask is ignored and only the address you typed is tested, so `--src 10.0.3.0/24` predicts for the single address `10.0.3.0` and tells you nothing about `10.0.3.50`. It does not evaluate the range.

  _Use to check the effect of a proposed firewall change before applying it live._

  ```bash
  unifi-network-cli rule-predict --src 10.0.3.50 --dst 10.0.0.1 --port 443 --json
  ```

## Paginated reads default to 25 rows

Nearly every paginated read takes `--limit` (**default 25**, except
`sites hotspot get-vouchers` which defaults to 100) and `--all`. This is not limited to
the `sites ...` subtree: `countries`, `pending-devices`, `dpi get-application-categories`,
and `dpi get-applications` behave the same way. Offset pagination emits **no truncation
warning**, so page 1 comes back looking like the whole answer. Any question phrased as
"every" or "all" needs `--all`:

```bash
unifi-network-cli sites devices get-adopted-overview-page <siteId> --all
unifi-network-cli sites firewall get-policies <siteId> --all
```

## Before any local-mirror command: sync

`drift`, `newcomer`, `topology`, `guest report`, `rule-predict`, and `search` all read the
local SQLite mirror, and `port-audit` needs the synced device list before it can fetch port
detail. On a fresh install the mirror is empty:

```bash
unifi-network-cli sync
```

**An unsynced mirror looks like an empty network, not like an error.** Only the explicit
`--site <name>` form fails loudly (exit **3**, `site "..." not found in local mirror`).
Without `--site`  -  the natural call on a single-site gateway, since the flag defaults to
"the only synced site"  -  every one of these exits **0** with an empty payload and puts
the `run 'unifi-network-cli sync' first` hint on **stderr only**:

| Command (no `--site`, empty mirror) | Exit | stdout |
| --- | --- | --- |
| `newcomer --json` | 0 | `[]` |
| `port-audit --json` | 0 | `[]` |
| `topology --json` | 0 | `{"site":"","devices":[],"unattached_clients":[]}` |
| `guest report --json` | 0 | empty vouchers + guests |
| `drift --json` | 0 | `{"site":"","first_run":false,"changes":[]}` |
| `rule-predict --json` | 0 | `"matched":false` |

`drift` is the sharpest edge: it positively asserts `first_run:false, changes:[]`  -  "not a
first run, nothing changed"  -  for a gateway that was never synced. **Always run `sync`
first, and read stderr**; never report "nothing found" from stdout alone.

## Command Reference

**countries**  -  Manage countries

- `unifi-network-cli countries`  -  Returns ISO-standard country codes and names, used for region-based configuration or regulatory compliance.

**dpi**  -  Manage dpi

- `unifi-network-cli dpi get-application-categories`  -  Returns predefined Deep Packet Inspection (DPI) application categories used for traffic identification and filtering.
- `unifi-network-cli dpi get-applications`  -  Lists DPI-recognized applications grouped under categories. Useful for firewall or traffic analytics integration.

**info**  -  Manage info

- `unifi-network-cli info`  -  Retrieve general information about the UniFi Network application.

**pending-devices**  -  Manage pending devices

- `unifi-network-cli pending-devices`  -  Retrieve a paginated list of devices pending adoption, including basic device information.

**sites**  -  Endpoints for listing and managing UniFi sites within a local Network application.
Site ID is required for most other API requests.

- `unifi-network-cli sites`  -  Retrieve a paginated list of local sites managed by this Network application.


**This reference is partial.** It lists the top-level convenience commands only; the full
surface is 137 commands, including the entire `sites ...` subtree (devices, clients,
firewall, ACL, networks, DNS, WiFi, hotspot, switching, VPN, WANs, RADIUS,
traffic-matching-lists). Enumerate it with `unifi-network-cli sites --help`,
`unifi-network-cli api` for every endpoint by interface, or read
[guide.md](./guide.md).

### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
unifi-network-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. **Under `--agent` or `--json` the exit code is always 0**; test `matches` for emptiness instead.

## Recipes

### Find who just joined the network

```bash
unifi-network-cli newcomer --since 7d --json --select id,name,mac
```

Narrow a potentially large newcomer list down to just the fields needed to identify each device.

### Audit switch port headroom before adding a PoE device

```bash
unifi-network-cli port-audit --site default --json
```

Lists PoE status and free ports across every switch on the site.

### Check what a firewall change would match

```bash
unifi-network-cli rule-predict --src 10.0.3.50 --dst 10.0.0.1 --port 443 --json
```

Simulates rule evaluation order against the synced ruleset before making a live change.

## Auth Setup

Two environment variables are required:

- `UNIFI_API_KEY` - generate a local API key from the gateway's own UI (Settings -> Control Plane -> Integrations -> Create API Key).
- `UNIFI_GATEWAY_HOST` - the gateway's hostname or IP on your network, e.g. `10.0.0.1`. The CLI builds the base URL from it (`https://<host>/proxy/network`); the integration spec declares no absolute server, so there is no default to fall back on. Set `UNIFI_BASE_URL` instead to point at a non-standard endpoint directly.

The gateway's self-signed certificate is handled automatically for private, loopback, and link-local hosts, so the common case needs no extra configuration. There is no `--insecure` flag; the auto-detection matches only a literal RFC1918 / loopback / link-local IP or the exact string `localhost`  -  any DNS name (`unifi.lan` included) is treated as not-private, so a hostname or public IP will fail certificate verification and needs `UNIFI_INSECURE_SKIP_VERIFY=1` set deliberately. It also accepts 0/false to force verification back on when you have installed a real certificate.

Run `unifi-network-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  unifi-network-cli countries --agent --select code,name
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts. Note that most `sites ...` commands take required POSITIONAL arguments rather than flags (`sites firewall get-policies <siteId>`, `sites devices execute-port-action <siteId> <deviceId> <portIdx>`); there is no `--site-id` flag
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

- Use `--home <dir>` for one invocation, or set `UNIFI_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `UNIFI_CONFIG_DIR`, `UNIFI_DATA_DIR`, `UNIFI_STATE_DIR`, `UNIFI_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `UNIFI_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, and `novel-snapshots/` (drift's per-site snapshots and newcomer's first-seen file). `state` contains `teach.log`, `learnings.jsonl`, and the learn journal. The `.agentcookie-managed` marker sits beside `config.toml` in the CONFIG dir, not data. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `unifi-network-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "unifi": {
        "command": "unifi-network-mcp",
        "env": {
          "UNIFI_HOME": "/srv/unifi"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `UNIFI_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `UNIFI_HOME`, or `doctor` will not find credentials left under the former root.

## Automatic learning

This CLI ships a self-capturing learning loop. The CLI does its own bookkeeping: every invocation is journaled locally, a failed flag followed by a corrected retry auto-derives a `flag_alias` candidate, and a `teach` on a query family without a playbook auto-synthesizes a `playbook_candidate` from the session's journal. Your job is judgment only: `recall` first, act on surfaced candidates, `teach` the final answer, `playbook amend` when you observe a correction. You never record failures by hand.

### Step 1: `recall` before any discovery

Before list/search/drill commands on a new user question, run:

```bash
unifi-network-cli recall "<user's question>" --agent
```

The response envelope. NOTE: every field below is nested under a top-level `results`
object (alongside `meta`), so the hit array is `.results.results` and the flag is
`.results.found`  -  not `.found`:

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
      "next_action": ["<trial command>", "unifi-network-cli learnings confirm 12"] }
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
       materially more, record the divergence via `unifi-network-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `unifi-network-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., a device name that exists on two sites). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `unifi-network-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately:

```bash
unifi-network-cli teach --query "<user's question>" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. Cross-alias resolution uses the `entity_lookups` table. This CLI ships **no seed data** for it (the generated `initLearn` is a no-op), so it starts empty and fills only from `sync` and from explicit `teach-lookup` rows  -  a teach under one alias does not resolve another alias until a lookup row exists. Use `teach-lookup` when a device or site is known by more than one name.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
unifi-network-cli teach \
  --query "<user's question>" \
  --resource-type <type> \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
unifi-network-cli teach-playbook \
  --query "<user's question>" \
  --playbook-file ~/playbooks/<shape>.json \
  --notes-file ~/playbooks/<shape>-notes.md
```

Playbook files are JSON with `steps`, `entity_slots`, `expected_tool_calls`. Notes files are markdown carrying the gotchas verbatim. File-free callers (MCP-only agents) pass the same content inline: `--playbook-json` and `--playbook-notes` on the integrated `teach` form, `--playbook-json` and `--notes` on `teach-playbook`. On the integrated `teach` form, the playbook flags are optional - omit them entirely for a resource-only teach. On the standalone `teach-playbook` form, at least one of the playbook and notes flags must be set; both empty is rejected. Playbooks are keyed on the structural query family (entities stripped) so a recipe taught from one entity-shaped query applies to every other query of the same shape, with `slots_resolved` binding the live query's canonical at recall time.

When you DO find a playbook on a future recall, treat it as ground truth: replay the steps with `slots_resolved` substitutions, skip the discovery that the choreography already documents, and read `notes` before any step.

### Step 6: `playbook amend &` when your debug response identifies a correction

If your debug-protocol response identifies a concrete correction the notes or playbook should know  -  a workaround, an undocumented endpoint shape, a stale field name, observed schema drift, an empty-payload fallback  -  fire `playbook amend` BEFORE emitting your user-facing response. Same fire-and-forget posture as `teach`.

```bash
unifi-network-cli playbook amend \
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

`unifi-network-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `UNIFI_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
unifi-network-cli feedback "the --since flag is inclusive but docs say exclusive"
unifi-network-cli feedback --stdin < notes.txt
unifi-network-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `UNIFI_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `UNIFI_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename) |
| `webhook:<url>` | POST the output body to the URL (`application/json` or `application/x-ndjson` when `--compact`) |

Unknown schemes are refused with a plain-text error naming the supported set, exiting 1. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
unifi-network-cli profile save briefing --json
unifi-network-cli --profile briefing countries
unifi-network-cli profile list --json
unifi-network-cli profile show briefing
unifi-network-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 1 | Pre-run failure (e.g. an unknown `--deliver` scheme), raised before the typed codes apply |
| 5 | API error (upstream issue) |
| 6 | Partial failure (2xx with a partial-failure body; pass `--allow-partial-failure` to downgrade to a warning) |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `unifi-network-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`), subject to the
   safety model below  -  `--agent` implies `--yes`, and this CLI has no confirmation prompt
   on any gateway-mutating command.

## Safety model (read before running anything that mutates)

Full tiers and rationale: [governance.md](./governance.md). ([AGENTS.md](./AGENTS.md) is the
maintainer/operating contract, not a permission model.)
The short version an agent must honor:

- **The CLI never redacts output. Any command that can emit a stored or returned field
  verbatim can emit a secret.** Two secrets exist on this API: an SSID's cleartext
  `passphrase` (inside `securityConfiguration`) and a usable guest voucher `code`.
  Treat the output of anything that can surface them as credential-grade  -  do not run
  it unattended, log it, or paste it into context. Known carriers:
  - direct reads: `sites wifi get-broadcast-details` (passphrase),
    `sites hotspot get-voucher` / `get-vouchers` (codes);
  - mirror-derived, no gateway call: `guest report`, `search`, `analytics`
    (`--group-by code` on the `hotspot` type dumps every code);
  - writes that echo the secret back: `sites hotspot create-vouchers`,
    `sites wifi create-broadcast` / `update-broadcast`;
  - and `sync` itself, which persists voucher codes into `data.db` in cleartext.
  If you add or discover another command that reads the mirror, assume it belongs here.
- **Never auto-run these  -  ask a human first.** `sites devices remove` (unadopts AND
  **factory-resets** an online device), `sites devices execute-port-action` (can
  power-cycle PoE and drop whatever is plugged in), `sites clients
  execute-connected-action` (can force a client off the network), `sites devices adopt`,
  `sites devices execute-adopted-action`, and every `delete` command
  (`sites firewall delete-policy`, `sites firewall delete-zone`, `sites networks delete`,
  `sites acl-rules delete`, `sites dns delete-policy`, `sites wifi delete-broadcast`,
  `sites traffic-matching-lists delete`, `sites hotspot delete-voucher(s)`).
- **Every other gateway-mutating command is a routine config write**  -  any
  `create` / `update` / `patch` under `sites ...` (firewall policies and zones, ACL
  rules, networks, DNS policies, WiFi broadcasts, hotspot vouchers, **traffic-matching
  lists**, policy ordering). Preview with `--dry-run`, show the exact command, get
  approval, then run. If a command mutates and is not named above, treat it as this
  tier  -  never as a read.
- **`drift` and `newcomer` write local state.** `drift` advances its own snapshot every
  run, so running it twice makes the second run report no changes.

## MCP Server Installation

1. Install the MCP server (the same installer that ships the CLI also drops
   `unifi-network-mcp` on `$PATH`):
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/unifi-network/install.sh)
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add unifi-network-mcp -- unifi-network-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which unifi-network-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   unifi-network-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `unifi-network-cli <command> --help`.
