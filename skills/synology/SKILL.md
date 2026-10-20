---
name: synology
description: "Use when the user asks whether client Synology NAS backups succeeded, which machines have not backed up, or how client NAS storage is doing: mirrors Synology Active Backup for Business jobs and storage health from every registered client NAS (one DSM login each, two-factor supported) into one local database, then answers failed and missed runs, stale devices, which task protects a machine, why a task failed, success-rate trends and degraded pools, bad disks or nearly full volumes across all of them. Also reads Hyper Backup, storage, system health, logs, users and groups per NAS. Trigger phrases: `did last night's synology backups succeed`, `active backup for business failures`, `which machines have not backed up`, `which client nas is running out of space`, `check synology nas disk health`, `why did the synology backup fail`, `use synology`, `run synology-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Synology"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - synology-cli
    install:
      - kind: script
        bins: [synology-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/synology/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/synology/install.ps1
---

# Synology - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `synology-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/synology/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/synology/install.ps1 | iex
   ```
3. Verify: `synology-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `synology-cli` and `synology-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Talks to each Synology NAS directly with its own DSM login and mirrors Active Backup for Business job history plus volume, disk and SMART health into SQLite. After fleet add and fleet sync, fleet status, abb failures and abb stale answer the MSP morning check across every client NAS, with exit code 6 an RMM can alert on.

## When to Use This CLI

Use this CLI when an agent needs to know whether Synology Active Backup for Business jobs succeeded, which protected machines are behind, or whether NAS volumes and disks are healthy, on one NAS or across many client NAS units. It is built for MSP morning checks, weekly reviews and RMM alerting. Fleet reports read the local mirror, so run fleet sync first; it has not been verified against a live NAS by its authors.

## Anti-triggers

Do not use this CLI for:
- Restoring files or bare-metal images: use the Active Backup for Business portal or recovery media
- Deleting backup tasks or versions: the CLI deliberately does not expose them; use DSM
- Browsing or copying files on the NAS: use File Station or SMB
- Microsoft 365 or Google Workspace backups (Active Backup for Microsoft 365 / Google Workspace): different packages, not covered

## Unique Capabilities

### Fleet-wide backup assurance
- **`fleet status`**  -  One row per client NAS with backup outcomes, stale devices, worst storage state and sync age; exits 6 when any NAS is warning, critical or unknown (--no-fail exits 0).

  _Reach for this first when asked whether every client's backups and storage are OK; a NAS that stopped reporting shows UNKNOWN, never OK._

  ```bash
  synology-cli fleet status --agent
  ```
- **`abb failures`**  -  Every Active Backup task across the fleet whose latest run in a window failed, partially succeeded, was canceled, or never ran.

  _Use for the morning backup check instead of opening the ABB portal on each NAS._

  ```bash
  synology-cli abb failures --since 24h --agent
  ```
- **`abb stale`**  -  Protected PCs, servers and VMs whose last successful backup is older than a threshold, or that never succeeded.

  _Catches machines that silently stopped getting good backups, which no single ABB screen flags._

  ```bash
  synology-cli abb stale --older-than 36h --agent
  ```
- **`fleet sync`**  -  Mirror ABB job status and storage health from every NAS registered with fleet add into one local database, each NAS with its own login.

  _Run this before the fleet rollups; one unreachable NAS is recorded and does not abort the run._

  ```bash
  synology-cli fleet sync --agent
  ```

### Storage health
- **`fleet health`**  -  Degraded or crashed volumes and pools, disks with bad SMART status, hot disks and nearly full volumes across every NAS, with days-to-full when history exists.

  _Use for the weekly storage review across clients; exit code is non-zero when anything needs attention._

  ```bash
  synology-cli fleet health --full-pct 85 --disk-temp 55 --agent
  ```

### Drill-down
- **`abb triage`**  -  Why one ABB task failed: its newest non-success run, its detail lines and the ABB log from that window, errors first. Live call to one NAS; DSM returns log types and parameters, not message text.

  _Use after abb failures names a task, before re-running it._

  ```bash
  synology-cli abb triage "Office PCs" --nas acme --agent
  ```
- **`abb device`**  -  Find a machine by host name across every NAS and see which task protects it, its last success and its recent runs.

  _Answers the ticket question 'is this machine backed up, and when was the last good one?'_

  ```bash
  synology-cli abb device BOB-LAPTOP --agent
  ```
- **`abb trend`**  -  Per task or device: run count, success rate, median and p90 duration, and an overrun flag when the latest run is far slower than usual.

  _Spots tasks that keep partially succeeding or keep getting slower before they break._

  ```bash
  synology-cli abb trend --since 30d --agent
  ```

## Command Reference

**abb**  -  Active Backup for Business tasks, job results, logs and storage

- `synology-cli abb backup`  -  Start a backup now for one or more tasks
- `synology-cli abb cancel`  -  Cancel the running backup of one or more tasks
- `synology-cli abb hypervisors`  -  List the VMware and Hyper-V hypervisors registered in Active Backup for Business
- `synology-cli abb logs`  -  Read the Active Backup for Business activity log for the package, tasks and devices
- `synology-cli abb result-detail`  -  Show the detail log lines (errors, warnings, skipped paths) of one job result
- `synology-cli abb results`  -  List job results across tasks (backups plus version deletions and restores; filter {'job_action':1} for backups only)
- `synology-cli abb settings`  -  Show Active Backup for Business package settings and certificate information
- `synology-cli abb storage`  -  List Active Backup for Business storage destinations with dedup size, device counts and volume
- `synology-cli abb tasks`  -  List Active Backup for Business tasks with their last job result, protected devices, progress and next run time
- `synology-cli abb transfers`  -  List protected devices with every transfer (bytes, status, start, end) inside a time window

**folder**  -  Shared folders

- `synology-cli folder`  -  List shared folders, including the ActiveBackupforBusiness destination share

**group**  -  Local DSM groups

- `synology-cli group list`  -  List local DSM groups on the NAS, for example to find the administrators group
- `synology-cli group members`  -  List the accounts in one group, for example administrators

**hyperbackup**  -  Hyper Backup tasks (the NAS's own backup to another NAS, USB or cloud)

- `synology-cli hyperbackup task-status`  -  Show one Hyper Backup task's last result, last success, next run and health state without contacting the destination
- `synology-cli hyperbackup tasks`  -  List Hyper Backup tasks (the NAS's own backups to another NAS, USB or cloud) with their destination type

**log**  -  DSM system log

- `synology-cli log`  -  Read the DSM system log with level, date range and keyword filters

**package**  -  Installed DSM packages

- `synology-cli package`  -  List installed packages with version and running state; confirms Active Backup for Business is installed

**session**  -  DSM API discovery and session control

- `synology-cli session apis`  -  List every API namespace this NAS exposes with its version range and CGI path
- `synology-cli session logout`  -  Invalidate the current DSM session id so it can no longer be used on this NAS

**storage**  -  Volumes, storage pools, disks and SMART data

- `synology-cli storage disks`  -  List physical disks with model, serial number, temperature and SMART health status
- `synology-cli storage overview`  -  Show Storage Manager's disks, storage pools and volumes with their status in one call
- `synology-cli storage pools`  -  List storage pools with RAID type, status and member disks
- `synology-cli storage smart`  -  Show the SMART attribute table for one disk
- `synology-cli storage smart-schedule`  -  List the scheduled SMART tests (quick or extended) configured for the NAS's disks
- `synology-cli storage volumes`  -  List volumes with status, size, used space and file system

**system**  -  DSM system identity, health and load

- `synology-cli system dsm`  -  Show DSM product identity (model, version string, serial) as the web UI reports it
- `synology-cli system get`  -  Show model, serial number, DSM version, uptime and system temperature
- `synology-cli system health-check`  -  Show the aggregate system health verdict the DSM dashboard shows
- `synology-cli system network`  -  Show the NAS hostname, DNS servers, default gateway and network interface settings
- `synology-cli system ups`  -  Show the state of the uninterruptible power supply (UPS) attached to the NAS, if any
- `synology-cli system utilization`  -  Show current CPU, memory, disk and network utilization

**user**  -  Local DSM user accounts

- `synology-cli user`  -  List local user accounts, for example to audit who can log in to a client NAS


**fleet**  -  register client NAS units and mirror them

- `synology-cli fleet add <name> --url https://<nas>:5001 [--client NAME] [--env-prefix PREFIX] [--pin-cert | --tls-fingerprint SHA256 | --insecure] [--check]`  -  Register a NAS; its login comes from `<prefix>_ACCOUNT`/`<prefix>_PASSWORD`
- `synology-cli fleet list`  -  Registered NAS units, which credential env vars are set, TLS mode and last sync
- `synology-cli fleet remove <name>`  -  Unregister a NAS and purge its mirror rows
- `synology-cli auth login`  -  Log in to the `SYNOLOGY_BASE_URL` NAS and cache the session (add `SYNOLOGY_OTP_CODE` once for two-factor)

**abb**  -  fleet reports (read the local mirror; run `fleet sync` first)

- `synology-cli abb failures`  -  Every Active Backup task across the fleet whose latest run in a window failed, partially succeeded, was canceled, or never ran.

- `synology-cli abb stale`  -  Protected PCs, servers and VMs whose last successful backup is older than a threshold, or that never succeeded.

- `synology-cli abb triage`  -  Why one ABB task failed: its newest non-success run, its detail lines and the ABB log from that window, errors first. Live call to one NAS; DSM returns log types and parameters, not message text.

- `synology-cli abb device`  -  Find a machine by host name across every NAS and see which task protects it, its last success and its recent runs.

- `synology-cli abb trend`  -  Per task or device: run count, success rate, median and p90 duration, and an overrun flag when the latest run is far slower than usual.


**fleet**  -  fleet reports and sync

- `synology-cli fleet status`  -  One row per client NAS with backup outcomes, stale devices, worst storage state and sync age; exits 6 when any NAS is warning, critical or unknown (--no-fail exits 0).

- `synology-cli fleet sync`  -  Mirror ABB job status and storage health from every NAS registered with fleet add into one local database, each NAS with its own login.

- `synology-cli fleet health`  -  Degraded or crashed volumes and pools, disks with bad SMART status, hot disks and nearly full volumes across every NAS, with days-to-full when history exists.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
synology-cli which "machines with no recent backup"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Morning backup check across clients

```bash
synology-cli abb failures --since 24h --agent
```

After fleet sync, list every failed, partial or missed ABB run in the last day across client NAS units.

### Machines with no good backup in 3 days

```bash
synology-cli abb stale --older-than 72h --agent
```

Device-level staleness computed from job history, including devices that never succeeded.

### Narrow a verbose task list

```bash
synology-cli abb tasks --agent --select task_id,task_name,last_result.status,devices.host_name
```

ABB task rows carry dozens of fields; select only the outcome and the protected hosts.

### Why did a task fail

```bash
synology-cli abb triage "Office PCs" --nas acme --agent
```

Newest failed run, its detail lines and the ABB log from that window, errors first.

### Fleet storage review

```bash
synology-cli fleet health --full-pct 85 --agent
```

Degraded volumes, bad SMART disks, hot disks and nearly full volumes on every client NAS.

## Limitations

- Fleet reports (`fleet status`, `fleet health`, `abb failures`, `abb stale`, `abb device`, `abb trend`) read the local mirror: run `fleet sync` first. A NAS whose mirror is missing, failed or older than `--max-sync-age` appears as `nas_not_synced`/unknown, never as OK.
- With no NAS registered and no `SYNOLOGY_BASE_URL`, those reports exit 10 (config error) instead of printing an empty result.
- `abb failures` uses a 24h window (`fleet status`: 26h); a weekly task shows as `no_run_in_window` unless you widen `--since` (for example `--since 8d`).
- `abb` and `hyperbackup` commands need the Active Backup for Business / Hyper Backup package on the NAS (otherwise DSM error 102).
- DSM's Active Backup API is undocumented; the call shapes come from public open-source clients and were not verified against a live NAS by this CLI's authors.
- `abb_missing` means Active Backup for Business ran on a NAS before and no longer answers (DSM error 102); it counts as a finding. `abb_not_installed` (a NAS that never ran ABB) does not.
- `abb trend` sets `history_from` on rows whose window starts before the mirrored history; backfill with `fleet sync --nas <name> --full --since 30d`.
- After upgrading this CLI, every NAS reads as `nas_not_synced` until its next `fleet sync` (the mirror records which version wrote it). If DSM contradicts itself about a run (task list vs result listing), that NAS's sync fails rather than guessing.
- `fleet health` warns `temperature not reported` for a disk that reports no temperature (some SSDs and virtual DSM); `--disk-temp 0` skips the temperature check.

## Auth Setup

DSM has no API keys. The CLI logs in with SYNO.API.Auth using SYNOLOGY_ACCOUNT and SYNOLOGY_PASSWORD (sent as a form POST, never stored) against SYNOLOGY_BASE_URL (for example https://192.168.1.50:5001), sends the session id and SynoToken on every call, and logs in again when DSM reports an expired session. Run 'synology-cli auth login' once to test the login and cache the session. For a two-factor account run it with a fresh SYNOLOGY_OTP_CODE: the CLI caches the trusted-device token DSM returns, so the code is not needed again (SYNOLOGY_DEVICE_ID overrides the cached token). Fleet entries read <prefix>_ACCOUNT, <prefix>_PASSWORD and optional <prefix>_OTP_CODE / <prefix>_DEVICE_ID, and cache the trusted-device token per NAS the same way. DSM's self-signed certificate: pin it with SYNOLOGY_TLS_FINGERPRINT=<sha256> or 'fleet add --pin-cert'; SYNOLOGY_SKIP_TLS_VERIFY=1 trusts any certificate on a network you control.

Run `synology-cli doctor` to verify setup.

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
  synology-cli abb tasks --agent --select task_name,last_result.status
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **No confirmation prompts**  -  `abb backup`, `abb cancel` and `session logout` act on the NAS immediately with no confirmation prompt; preview with `--dry-run` first. `fleet add` and `fleet remove` are operator-only CLI commands, not MCP tools: they decide which URL each NAS password is sent to.

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. The fleet and abb report commands (`fleet status`, `fleet health`, `fleet list`, `abb failures`, `abb stale`, `abb device`, `abb trend`) wrap their rows in this envelope under `--agent`; with plain `--json` they print the bare row array. Live reads may be answered from a 5-minute response cache and are still labelled `live`; add `--no-cache` when you need the NAS's state right now. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set  -  piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Paths and state

Agents should treat the CLI's path resolver as part of the runtime contract:

- Use `--home <dir>` for one invocation, or set `SYNOLOGY_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `SYNOLOGY_CONFIG_DIR`, `SYNOLOGY_DATA_DIR`, `SYNOLOGY_STATE_DIR`, `SYNOLOGY_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `SYNOLOGY_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` holds `config.json`, profiles and the `fleet.json` NAS registry. `data` holds `credentials.toml`, `data.db`, the `fleet.db` mirror and `session-<scheme>-<host>_<port>-<hash>.json` per NAS (DSM session id, SynoToken and two-factor trusted-device token, mode 0600). `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `synology-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "synology": {
        "command": "synology-mcp",
        "env": {
          "SYNOLOGY_HOME": "/srv/synology"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `SYNOLOGY_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `SYNOLOGY_HOME`, or `doctor` will not find credentials left under the former root.

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
synology-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "synology-cli learnings confirm 12"] }
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
       materially more, record the divergence via `synology-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `synology-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `synology-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
synology-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
synology-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
synology-cli teach-playbook \
  --query "$QUERY" \
  --playbook-file ~/playbooks/<shape>.json \
  --notes-file ~/playbooks/<shape>-notes.md
```

Playbook files are JSON with `steps`, `entity_slots`, `expected_tool_calls`. Notes files are markdown carrying the gotchas verbatim. File-free callers (MCP-only agents, where the file flags are not offered) pass the same content inline: `--playbook-json` and `--playbook-notes` on the integrated `teach` form, `--playbook-json` and `--notes` on `teach-playbook`. On the integrated `teach` form, the playbook flags are optional - omit them entirely for a resource-only teach. On the standalone `teach-playbook` form, at least one of the playbook and notes flags must be set; both empty is rejected. Playbooks are keyed on the structural query family (entities stripped) so a recipe taught from one entity-shaped query applies to every other query of the same shape, with `slots_resolved` binding the live query's canonical at recall time.

When you DO find a playbook on a future recall, treat it as ground truth: replay the steps with `slots_resolved` substitutions, skip the discovery that the choreography already documents, and read `notes` before any step.

### Step 6: `playbook amend &` when your debug response identifies a correction

If your debug-protocol response identifies a concrete correction the notes or playbook should know  -  a workaround, an undocumented endpoint shape, a stale field name, observed schema drift, an empty-payload fallback  -  fire `playbook amend` BEFORE emitting your user-facing response. Same fire-and-forget posture as `teach`. Pass the query and note as argv/MCP arguments, or write each with a non-shell file tool and read them back (`QUERY=$(cat ...)`, `NOTE=$(cat ...)`). Do not interpolate either string into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
NOTE=$(cat /path/to/note.txt)
synology-cli playbook amend \
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

`synology-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `SYNOLOGY_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
synology-cli feedback "the --since flag is inclusive but docs say exclusive"
synology-cli feedback --stdin < notes.txt
Get-Content notes.txt | synology-cli feedback --stdin   # PowerShell
synology-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `SYNOLOGY_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `SYNOLOGY_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command except `completion` accepts `--deliver <sink>` (redirect a completion script to a file instead; the `--help` and `--version` flags print to stdout only, while the `help` and `version` commands deliver). A command that fails still delivers what it printed, so `fleet status`, `fleet health` and the `abb` reports deliver their rows when they exit 6 with findings. `profile save` validates a `--deliver` sink and records it for later `--profile` runs rather than delivering its own confirmation. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
synology-cli profile save briefing --json
synology-cli --profile briefing folder
synology-cli profile list --json
synology-cli profile show briefing
synology-cli profile delete briefing --yes
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
| 6 | Findings: a `fleet`/`abb` report ran and something needs attention (`--no-fail` exits 0) |
| 10 | Config error (`fleet`/`abb` reports and `fleet sync`: no NAS configured) |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `synology-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add synology-mcp --env SYNOLOGY_BASE_URL=https://<nas>:5001 --env SYNOLOGY_ACCOUNT=<account> --env SYNOLOGY_PASSWORD=<password> -- synology-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which synology-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   synology-cli abb failures --since 24h --agent
   ```
4. If ambiguous, drill into subcommand help, for example `synology-cli abb --help` or `synology-cli fleet add --help`.
