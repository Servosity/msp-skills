---
name: windows-server
description: "Agentless Windows Server health checks over WinRM: one verdict per server, a diffable inventory, and no arbitrary PowerShell. Trigger phrases: `is the file server healthy`, `why is this service stopped on the server`, `who added a local admin on the server`, `what changed on our windows servers this week`, `check expiring certificates on the server`, `use windows-server`, `run windows-server`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Windows Server"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - windows-server-cli
    install:
      - kind: script
        bins: [windows-server-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/windows-server/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/windows-server/install.ps1
---

# Windows Server Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `windows-server-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/windows-server/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/windows-server/install.ps1 | iex
   ```
3. Verify: `windows-server-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `windows-server-cli` and `windows-server-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Every check is a fixed, parameterized PowerShell script sent over WinRM HTTPS with certificate verification always on. health rolls the checks into one verdict per server, inventory diff shows drift between runs, and services down explains why a service stopped. The MCP server can never pick a host or run its own script.

## When to Use This CLI

Use this CLI to answer whether a Windows Server is healthy, why a service is down, which errors are new, and what changed in roles, shares, local administrators, certificates and updates between inventory snapshots. health, services down, eventlog new, local-admins changes and inventory snapshot cover every server in the config; the single-resource reads (certificates, volumes, system and the rest), services restart and services start act on one server: the one --server names, or the one marked default = true. It is also the safe way to restart or start a single service with a preview and confirmation.

## Anti-triggers

Do not use this CLI for:
- Running arbitrary PowerShell or commands; it has no script surface by design
- Changing server configuration (registry, firewall, roles, users or group membership), or stopping or disabling a service; the only changes it makes are starting or restarting one service
- Installing Windows updates, rebooting servers, or administering Active Directory, DNS or DHCP
- Linux or other non-Windows hosts, or workstations managed by an RMM agent; it targets Windows servers reachable over WinRM

## Unique Capabilities

### Safe actions
- **`services restart`**  -  Restart one running service with services restart, or start a stopped automatic one with services start; each previews first and returns the service state before and after.

  _Reach for this when a ticket needs one specific service back up; it previews by default and only acts with confirmation._

  ```bash
  windows-server-cli services restart Spooler --server fs01 --agent
  ```

### Daily server check
- **`health`**  -  One ok, warn, crit or unknown verdict per server, with the reason for every failing check and how long it has been failing.

  _Use this first for any 'is this server healthy' question instead of running checks one by one._

  ```bash
  windows-server-cli health --agent
  ```
- **`services down`**  -  Stopped automatic services on every server, each paired with the Service Control Manager event that explains the stop.

  _Use this on a 'server is down' ticket before restarting anything._

  ```bash
  windows-server-cli services down --agent
  ```
- **`eventlog new`**  -  System and Application errors and warnings whose source and event ID are new on a server compared with its own history.

  _Use this during triage to separate a new failure from an error that fires every night._

  ```bash
  windows-server-cli eventlog new --window 24h --baseline 7d --agent
  ```

### Change tracking
- **`inventory diff`**  -  What changed in roles, features, shares, local administrators, certificates, installed updates and VMs between the two newest inventory snapshot runs.

  _Use this for drift reviews and change tickets: it names exactly what was added, removed or changed._

  ```bash
  windows-server-cli inventory diff --agent
  ```
- **`local-admins changes`**  -  Who added or removed each local Administrators member, when, and whether they are still a member.

  _Use this when a new local admin appears and you need the author and time._

  ```bash
  windows-server-cli local-admins changes --hours 168 --agent
  ```
- **`events list`**  -  Poll health-state changes between health runs with a sequence cursor.

  _Use this on every polling cycle to learn what changed since your last cursor._

  ```bash
  windows-server-cli events list --since 0 --agent
  ```

## Command Reference

Single-resource reads run one fixed script on one server: the one `--server` names, or the one marked `default = true`.

- `windows-server-cli certificates`: List certificates in a LocalMachine certificate store with expiry and days left (`--store`, `--expiring-days`)
- `windows-server-cli defender`: Read whether Defender real-time protection is on and how old its signatures are
- `windows-server-cli eventlog`: List recent critical, error and warning events from the System or Application log (`--log`, `--level`, `--hours`, `--max`)
- `windows-server-cli features`: List installed Windows Server roles, role services and features
- `windows-server-cli hyperv-vms`: List Hyper-V virtual machines with state, uptime, checkpoints and replication health
- `windows-server-cli installed-updates`: List installed Windows updates (hotfixes) with install date
- `windows-server-cli local-admins`: List the direct members of the local Administrators group with their SIDs
- `windows-server-cli pending-updates`: List software updates the Windows Update Agent finds applicable but not installed (takes 30-60 seconds)
- `windows-server-cli scheduled-tasks`: List scheduled tasks with state, last run time and last result code (`--failed`, `--include-microsoft`)
- `windows-server-cli services list`: List Windows services with state, start mode and service account (`--state`, `--start-mode`, `--stopped-automatic`)
- `windows-server-cli services get <name>`: Read one Windows service by its service name
- `windows-server-cli shares`: List SMB shares with path and whether each is an administrative share
- `windows-server-cli system`: Read the server's operating system, last boot time, uptime and whether a reboot is pending
- `windows-server-cli time-sync`: Read the Windows Time service state, time source, stratum and last successful sync
- `windows-server-cli volumes`: List fixed-disk volumes with size, free space and percent free

Fleet commands run on every configured server unless `--server` names one:

- `windows-server-cli health`: One ok, warn, crit or unknown verdict per server, with the reason for every failing check and how long it has been failing. Records each run in the local store.
- `windows-server-cli services down`: Stopped automatic services on every server, each paired with the Service Control Manager event that explains the stop (`--days`)
- `windows-server-cli eventlog new`: System and Application errors whose source and event ID are new on a server compared with its own history (`--window`, `--baseline`)
- `windows-server-cli local-admins changes`: Who added or removed each local Administrators member, when, and whether they are still a member (`--hours`)
- `windows-server-cli inventory snapshot`: Record the inventory of every server in the local store for inventory diff

Local store only (no server is contacted):

- `windows-server-cli inventory diff`: What changed in roles, features, shares, local administrators, certificates, installed updates and VMs between the two newest inventory snapshot runs.
- `windows-server-cli events list`: Poll health-state changes between health runs with a sequence cursor (`--since`, `--type`, `--limit`)

Service actions act on one server and only preview unless you pass `--yes` (CLI) or `confirm: true` (MCP):

- `windows-server-cli services restart <service-name>`: Restart one running service. Refused: WinRM and the services it depends on, a service with running dependents, and a service that is not running.
- `windows-server-cli services start <service-name>`: Start one stopped automatic-start service. Refused: a service that is not stopped, a manual or disabled service, and WinRM and the services it depends on.

Setup:

- `windows-server-cli servers list`: List configured servers and where each one's credential comes from (never the value)
- `windows-server-cli servers login <server>`: Store a credential for one server; the password is read from stdin, and `--username` defaults to the server's configured username
- `windows-server-cli servers logout <server>`: Remove one server's stored credential
- `windows-server-cli auth set-credentials <username> <password>`: Save the shared account used by every server without its own login (the password is an argument, so prefer `servers login` or `WINDOWS_SERVER_PASSWORD`)
- `windows-server-cli auth status`: Show whether a shared credential is configured
- `windows-server-cli auth logout`: Clear the stored shared credential (per-server logins stay until `servers logout`)

### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
windows-server-cli which "expiring certificates"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match, so fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Daily server check, narrowed

```bash
windows-server-cli health --agent --select server,verdict,checks.name,checks.status,checks.reason
```

One verdict per server with only the fields an agent needs from each check.

### Down ticket

```bash
windows-server-cli services down --server fs01 --agent
```

Lists stopped automatic services on fs01 with the Service Control Manager event behind each stop.

### Restart after preview

```bash
windows-server-cli services restart Spooler --server fs01 --yes
```

Run the same command without --yes first: it prints the preview and exits 2 with nothing changed. With --yes it restarts the Print Spooler on fs01 and returns state_before and state_after.

### Weekly drift review

```bash
windows-server-cli inventory diff --agent
```

Added, removed and changed inventory rows between the two newest inventory snapshot runs; run inventory snapshot on a schedule to feed it.

### Poll health changes

```bash
windows-server-cli events list --since 0 --agent
```

Returns health-state change events and the next cursor to poll from; a server's first health run records only the checks already failing.

## Auth Setup

Windows Server has no API key. The CLI signs in to WinRM over HTTPS (port 5986 by default) with a domain or local account. Each server is a [[servers]] entry in the config file (default ~/.config/windows-server-cli/config.toml; override with --config or WINDOWS_SERVER_CONFIG): name, host, optional port, auth (ntlm, the default, or basic), username, and cert_sha256 or ca_file when its certificate does not chain to a system root. Other users must not be able to change that file (chmod 600), or the CLI refuses its servers. Store a password per server with servers login <name>, which reads it from stdin, or share one account through WINDOWS_SERVER_USERNAME and WINDOWS_SERVER_PASSWORD (auth set-credentials also saves the shared account, but takes the password as an argument, so it lands in shell history). Passwords shorter than 8 characters are refused. With NTLM, a local account other than the built-in Administrator gets HTTP 401 unless LocalAccountTokenFilterPolicy is 1 on the server: use a domain account, or auth = "basic" over HTTPS. Keep passwords out of config.toml; a legacy server_password there is still read for compatibility.

Run `windows-server-cli doctor` to verify setup.

## Server setup

A server entry looks like this (`--server` takes the `name`, never a host):

```toml
[[servers]]
name = "fs01"
host = "fs01.corp.example"
auth = "ntlm"                  # or "basic"
username = "CORP\\svc-monitor"
cert_sha256 = "4B:EF:..."      # or ca_file = "/path/to/ca.pem"
default = true
```

Then store the password from stdin and check the result:

```bash
windows-server-cli servers login fs01 --username 'CORP\svc-monitor' < password.txt
windows-server-cli servers list --agent
windows-server-cli health --server fs01 --agent
```

`windows-server-cli doctor` shows resolved paths, the configured servers and whether each one has a credential (shared or from `servers login`), and local data. It does not contact any server or check a certificate pin (its api row reads `not verified here`), so use `servers list` and `health --server <name>` to test a server.

## Limits to know before you run

- **Previews exit 2.** `services restart` and `services start` without `--yes` (or `confirm: true` over MCP) print the preview and exit 2 with nothing changed; a refused action also exits 2. Exit 0 means the change happened.
- **One server or every server.** health, services down, eventlog new, local-admins changes and inventory snapshot read every configured server. The single-resource reads, `services restart`, `services start` and `sync` act on one server; with several servers and none marked `default = true`, pass `--server`.
- **History needs earlier runs.** `events list` records what changed between `health` runs; a server's first run records only the checks already failing. `inventory diff` needs two `inventory snapshot` runs and exits 3 ("no inventory recorded yet") before the first.
- **Optional roles exit 3.** `hyperv-vms` on a server without the Hyper-V role, and `defender` on one without Microsoft Defender Antivirus, exit 3 (not found) and name what is missing; `health` reports those checks as n/a.
- **Audit policy.** `local-admins changes` only sees changes when the "Security Group Management" audit subcategory logs Success; otherwise each server returns a row with a warning. The policy is read from `auditpol` output, which is in the server's display language: on a non-English server the warning can appear although auditing is on.
- **Stored passwords follow the config file and the address.** `servers login` saves a password for that server name, under that config file, at its configured host and port; another config's server of the same name never gets it. Move the config, or change the host or port, and run `servers login` again.
- **No local data.** With `WINDOWS_SERVER_NO_LOCAL_DATA=1`, `health` prints its verdicts without recording them (no `failing_since` history, no events), and `inventory snapshot` is refused.
- **Basic needs HTTPS.** `auth = "basic"` with `transport = "http"` is refused: the password would cross the network in clear text.
- **Slow checks.** `pending-updates` takes 30-60 seconds, and `health` runs the same search.
- **MCP read-only mode.** With `WINDOWS_SERVER_MCP_READ_ONLY=1`, `health` and `inventory snapshot` are not offered (they write the local store), nor are `services restart` and `services start`.
- **MCP read tools have no server argument.** `certificates_list`, `volumes_list` and the other single-resource read tools always read the default server. The mirrored tools (`health`, `services_down`, `eventlog_new`, `local_admins_changes`, `services_restart`, `services_start`) take a `server` name.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, sync, and `--deliver` paths:

- `--json`: one JSON document on stdout (sync progress events go to stderr)
- `--compact`: keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain`: tabular rows (collection envelopes unwrap to the row array). `--csv` prefixes `'` on a cell or header key that starts with `=`, `+`, `-`, `@`, tab, CR, or LF (fullwidth `＝＋－＠` too) so a spreadsheet does not run it as a formula; a plain signed number such as `-5` or `+2.5e3` is unchanged.
- `--quiet`: one identity value per row, no envelope

- **Pipeable**: JSON on stdout, errors on stderr
- **Filterable**: `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on large results:

  ```bash
  windows-server-cli certificates --agent --select days_left,dns_names,has_private_key
  ```
- **Previewable**: `--dry-run` contacts no server. Hand-written commands print a "would run ... on <target>" summary; the single-resource reads print the placeholder request they would answer (`GET https://winrm.invalid/...`). For `services restart` and `services start`, the real preview is running the command without `--yes`.
- **Offline-friendly**: `sync`, `search` and `analytics` work from the local SQLite store; `sync` reads one server (`--server` or the default)
- **Non-interactive**: never prompts, every input is a flag
- **Reads only, with two exceptions**: `services restart` and `services start` change one service on one server, and only with `--yes` (CLI) or `confirm: true` (MCP). Everything else reads. Do not use this CLI for other changes.

### Response envelope

Commands that read from the local store or a server wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set; piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Paths and state

Agents should treat the CLI's path resolver as part of the runtime contract:

- Use `--home <dir>` for one invocation, or set `WINDOWS_SERVER_HOME=<dir>` to relocate all four path kinds under one root. `--config <file>` or `WINDOWS_SERVER_CONFIG` points at one config file instead.
- Use per-kind env vars only when a specific kind must diverge: `WINDOWS_SERVER_CONFIG_DIR`, `WINDOWS_SERVER_DATA_DIR`, `WINDOWS_SERVER_STATE_DIR`, `WINDOWS_SERVER_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `WINDOWS_SERVER_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains `config.toml` (with the `[[servers]]` list) and profiles. `data` contains `credentials.toml` (the shared account), `server-credentials.toml` (per-server logins, mode 0600) and `data.db`. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable cache files.
- Stored secrets live in `credentials.toml` and `server-credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `windows-server-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "windows-server": {
        "command": "windows-server-mcp",
        "env": {
          "WINDOWS_SERVER_HOME": "/srv/windows-server"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `WINDOWS_SERVER_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `WINDOWS_SERVER_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store, and `health` and `inventory snapshot` record their runs there; server data is never written to the HTTP response cache. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `WINDOWS_SERVER_NO_LOCAL_DATA=1` stops reads from being written to the store, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `WINDOWS_SERVER_DATA_SOURCE` sets the `--data-source` default.

## Automatic learning

This CLI ships a self-capturing learning loop. The CLI does its own bookkeeping: every invocation is journaled locally, a failed flag followed by a corrected retry auto-derives a `flag_alias` candidate, and a `teach` on a query family without a playbook auto-synthesizes a `playbook_candidate` from the session's journal. Your job is judgment only: `recall` first, act on surfaced candidates, `teach` the final answer, `playbook amend` when you observe a correction. You never record failures by hand.

### Step 1: `recall` before any discovery

Before list/search/drill commands on a new user question, pass the question as an argv or MCP tool argument to `recall --agent`. Do not interpolate user-controlled text into a shell command line.

Quoted `recall "<question>"` breaks on an apostrophe, which is ordinary English. A quoted heredoc breaks when a body line equals the delimiter, and that delimiter is published in these docs. Write the question with a non-shell file-writing tool, then read it back as data:

```bash
# Write the question verbatim with your file-writing tool (no shell involved).
# Command substitution on a file only ever yields data; the shell never
# parses the file's bytes as syntax.
QUERY=$(cat /path/to/question.txt)
windows-server-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "windows-server-cli learnings confirm 12"] }
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
       materially more, record the divergence via `windows-server-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `windows-server-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities, so direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (a teach under one spelling of a name satisfying a recall under another). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (one short name that could mean two different services). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet. This CLI seeds no entity lookups (server and service names are used verbatim), so treat it as a cold start.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall: argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
windows-server-cli teach --query "$QUERY" --resource-type services --resource fs01/Spooler
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time; this CLI seeds none (server and service names are used verbatim), so teach under the exact names you will query.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, output gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
windows-server-cli teach \
  --query "$QUERY" \
  --resource-type services \
  --resource fs01/Spooler \
  --playbook-file ~/playbooks/service-down.json \
  --playbook-notes-file ~/playbooks/service-down-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
windows-server-cli teach-playbook \
  --query "$QUERY" \
  --playbook-file ~/playbooks/service-down.json \
  --notes-file ~/playbooks/service-down-notes.md
```

Playbook files are JSON with `steps`, `entity_slots`, `expected_tool_calls`. Notes files are markdown carrying the gotchas verbatim. File-free callers (MCP-only agents) pass the same content inline: `--playbook-json` and `--playbook-notes` on the integrated `teach` form, `--playbook-json` and `--notes` on `teach-playbook`. On the integrated `teach` form, the playbook flags are optional - omit them entirely for a resource-only teach. On the standalone `teach-playbook` form, at least one of the playbook and notes flags must be set; both empty is rejected. Playbooks are keyed on the structural query family (entities stripped) so a recipe taught from one entity-shaped query applies to every other query of the same shape, with `slots_resolved` binding the live query's canonical at recall time.

When you DO find a playbook on a future recall, treat it as ground truth: replay the steps with `slots_resolved` substitutions, skip the discovery that the choreography already documents, and read `notes` before any step.

### Step 6: `playbook amend &` when your debug response identifies a correction

If your debug-protocol response identifies a concrete correction the notes or playbook should know (a workaround, an undocumented output shape, a stale field name, observed schema drift, an empty-result fallback), fire `playbook amend` BEFORE emitting your user-facing response. Same fire-and-forget posture as `teach`. Pass the query and note as argv/MCP arguments, or write each with a non-shell file tool and read them back (`QUERY=$(cat ...)`, `NOTE=$(cat ...)`). Do not interpolate either string into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
NOTE=$(cat /path/to/note.txt)
windows-server-cli playbook amend \
  --query "$QUERY" \
  --add-note "$NOTE"
# (append shell `&` to background it)
```

What counts as worth amending: a behavior you OBSERVED this session that future-you would benefit from knowing. Examples worth amending:

- A workaround for a CLI surface that silently drops or misorders a flag.
- An undocumented output shape (a field nested two levels deeper than the docs claim).
- Observed schema drift (a field renamed, or a value a newer Windows build returns in a different form).

What does NOT belong in notes:

- The server-specific answer to the user's question. That's the response, not a learning.
- Per-server or per-row data the playbook already retrieves at runtime.
- Statements that paraphrase what the existing notes already say.

The amend command appends to the family's existing notes with a timestamped marker (`[amend YYYY-MM-DDTHH:MMZ]: <text>`). Multiple amends accumulate; the audit trail is visible. If no playbook exists yet for the family, amend creates a notes-only one (so cold-start corrections still land).

#### PII discipline for amend notes

`playbook amend` notes are designed to potentially flow upstream as shared knowledge in future versions of the Printing Press. Keep them clean of user-identifying content so the upstream-contribution path stays open without retroactive scrubbing:

- **Do NOT embed** paths to user filesystems, passwords or tokens, user email addresses, user GitHub handles, or specific query histories tied to a single user.
- **Acceptable**: output shapes, undocumented field names, WinRM or script gotchas, observed schema drift, workarounds for CLI surfaces.

If a correction is only meaningful with user-specific context, it belongs in a personal note, not in the playbook amend.

### Measuring the loop

`windows-server-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `WINDOWS_SERVER_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
windows-server-cli feedback "the --since flag is inclusive but docs say exclusive"
windows-server-cli feedback --stdin < notes.txt
windows-server-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `WINDOWS_SERVER_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `WINDOWS_SERVER_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
windows-server-cli profile save briefing --json
windows-server-cli --profile briefing certificates
windows-server-cli profile list --json
windows-server-cli profile show briefing
windows-server-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success; for `services restart` and `services start`, the change was made and the service is Running |
| 2 | Usage error (bad flag or argument, a `--server` value that is not a server name; on hand-written commands also an unknown server name, or several servers with none marked `default = true`), a `services restart`/`start` preview without `--yes`, or a refused action |
| 3 | Not found: an unknown service name, or `inventory diff` before any snapshot ("no inventory recorded yet") |
| 4 | WinRM rejected the credentials, or the account was denied on the server; single-resource reads also return 4 when the server has no credential |
| 5 | WinRM or script failure on the server, or the service is not Running after a restart or start; single-resource reads also return 5 for config problems such as "no servers configured" |
| 10 | Config error on hand-written commands: no servers configured, a config file that cannot be read or that other users can change, or a server with no credential |

Exit 7 (rate limited) is not produced: a throttled WinRM listener surfaces as exit 5. Commands that read every server (health, services down, eventlog new, local-admins changes, inventory snapshot) print one row per server and exit 0 when at least one server answered; when none did, they exit with the first server's code.

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `windows-server-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add windows-server-mcp -- windows-server-mcp
   ```
3. Verify: `claude mcp list`

The MCP server reads the same config file and credential stores as the CLI, so add the `[[servers]]` entries and run `servers login` first (see Auth Setup). It never takes a host: the mirrored commands (`health`, `services_restart` and the rest) take a configured `server` name, the single-resource read tools (`certificates_list`, `volumes_list` and the rest) take none and always read the default server, and `--allow-http` is refused under MCP. Service actions need `confirm: true`.

For a remote host, `windows-server-mcp --transport http` requires `WINDOWS_SERVER_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `WINDOWS_SERVER_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `WINDOWS_SERVER_MCP_READ_ONLY=1` removes every tool not marked read-only: `services_restart`, `services_start`, `health`, `inventory_snapshot`, `sync`, `export`, `workflow_archive`, and the learning writers (`teach`, `teach_playbook`, `teach_pattern`, `teach_lookup`, `playbook_amend`, `learnings_confirm`, `learnings_forget`, `learnings_reject`). There is no tool that runs an arbitrary script.

## Direct Use

1. Check if installed: `which windows-server-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag, for example:
   ```bash
   windows-server-cli services down --agent
   ```
4. If ambiguous, drill into subcommand help, for example `windows-server-cli services down --help`.
