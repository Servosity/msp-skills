---
name: domain-cert-tracker
description: "Every client's domains and certificates in one inventory: what expires, unlocks or changes. Trigger phrases: `which client domains expire this month`, `check the certificate on this host`, `did any client domain lose its registrar lock`, `new certificates for our clients' domains`, `check SPF DMARC and CAA for a domain`, `use domain-cert-tracker`, `run domain-cert-tracker`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Servosity"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - domain-cert-tracker-cli
    install:
      - kind: script
        bins: [domain-cert-tracker-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domain-cert-tracker/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domain-cert-tracker/install.ps1
---

# Domain and Certificate Tracker - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `domain-cert-tracker-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domain-cert-tracker/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domain-cert-tracker/install.ps1 | iex
   ```
3. Verify: `domain-cert-tracker-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `domain-cert-tracker-cli` and `domain-cert-tracker-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Keep a per-client list of domains and hosts, then let sweep check registrations (RDAP), Certificate Transparency, live TLS and DNS for all of them and diff against the last run. report, renewals, issuers audit and hosts discover answer the weekly questions from the local store, and events list hands each change to an agent once. No account or credential is needed anywhere.

## When to Use This CLI

Use this CLI when an MSP needs to know, across many client companies, which domains and TLS certificates are about to expire, which registrations lost their transfer lock, which certificates appeared in Certificate Transparency that nobody ordered, and what DNS changed. It is the right tool for the nightly sweep and the Monday estate check, and for a quick live TLS or DNS answer on a ticket.

## Anti-triggers

Do not use this CLI for:
- Do not use it to renew, buy or transfer a domain or certificate; use the registrar or the CA's ACME client.
- Do not use it to scan internal or private hosts; it refuses non-public addresses by design.
- Do not use it for uptime or HTTP response monitoring; it checks registrations, certificates and DNS, not availability.
- Do not use it to edit DNS records; use the DNS host's connector or portal.

## Unique Capabilities

### Per-client estate
- **`clients list`**  -  Keep each client's domains and hosts in one local inventory that every sweep and report walks.

  _Reach for it first: sweep, report, renewals and hosts discover all iterate this inventory._

  ```bash
  domain-cert-tracker-cli clients list --agent
  ```
- **`report`**  -  See, per client, what expires within N days, which registrations are not transfer-locked, new CT certificates and DNS drift, plus what the last sweep could not check.

  _Use it for the weekly estate check; its unverified section keeps a failed check from reading as a clean result._

  ```bash
  domain-cert-tracker-cli report --days 30 --agent
  ```
- **`renewals`**  -  Find hosts still serving an old certificate after a newer one was logged, or past their renewal point with no successor in CT.

  _As lifetimes fall toward 47 days, this is where silently failed renewal automation shows up first._

  ```bash
  domain-cert-tracker-cli renewals --agent
  ```
- **`issuers audit`**  -  Separate certificates from issuers a domain's CAA does not permit, or a first-ever issuer, from routine renewals.

  _Use it to triage the week's new-certificate events so shadow IT and mis-issuance stand out from renewals._

  ```bash
  domain-cert-tracker-cli issuers audit --days 7 --agent
  ```
- **`hosts discover`**  -  List hostnames that have certificates in CT for a client's domains but are not in that client's inventory.

  _Run it at onboarding and weekly so the forgotten subdomain is watched before its certificate expires._

  ```bash
  domain-cert-tracker-cli hosts discover --agent
  ```

### Sweep and change feed
- **`events list`**  -  Read typed events from each sweep diff (domain.expiring, cert.expiring, ct.new_cert, dns.changed, registrar.unlocked) after a cursor.

  _Poll it after the nightly sweep to open tickets once per change instead of re-reporting the whole estate._

  ```bash
  domain-cert-tracker-cli events list --since 0 --agent
  ```
- **`sweep`**  -  Check every inventoried domain's registration, new CT certificates and DNS, and every inventoried host's live TLS certificate, in one run; then store the results and diff them against the last run.

  _Schedule it nightly; it is what fills report, renewals and the events feed._

  ```bash
  domain-cert-tracker-cli sweep --agent
  ```

### Live checks
- **`tls probe`**  -  Handshake with a live host and get its chain, expiry, SANs, key, issuer, hostname match and OCSP must-staple as JSON.

  _Use it on a certificate-warning ticket to see exactly what one host serves right now._

  ```bash
  domain-cert-tracker-cli tls probe example.com --agent
  ```
- **`dns check`**  -  Get NS, MX, SPF, DMARC, CAA and DNSSEC for a domain as one JSON document through the system resolver.

  _Use it for deliverability and onboarding reviews instead of several one-record lookups._

  ```bash
  domain-cert-tracker-cli dns check example.com --agent
  ```

## Command Reference

**autnum**  -  Look up autonomous system numbers over RDAP

- `domain-cert-tracker-cli autnum <asn>`  -  Look up who holds an autonomous system number over RDAP

**bootstrap**  -  Read the IANA RDAP bootstrap registry

- `domain-cert-tracker-cli bootstrap`  -  Read the IANA list of which RDAP server answers for each top-level domain

**certificates**  -  Search Certificate Transparency for certificates naming a domain

- `domain-cert-tracker-cli certificates --domain <domain>`  -  Search the Certificate Transparency log index for every certificate naming a domain

**domain**  -  Look up domain registrations over RDAP

- `domain-cert-tracker-cli domain <name>`  -  Look up a domain's registrar, expiry, lock statuses, nameservers and DNSSEC over RDAP

**ip**  -  Look up IP address networks over RDAP

- `domain-cert-tracker-cli ip <address>`  -  Look up which network and registry hold an IP address over RDAP

**nameserver**  -  Look up nameserver host objects over RDAP

- `domain-cert-tracker-cli nameserver <name>`  -  Look up a nameserver host object and its addresses over RDAP


**clients**


- `domain-cert-tracker-cli clients list`  -  List every client with its inventoried domains and hosts (read-only).
- `domain-cert-tracker-cli clients set <client> --domain <d> --host <h[:port]> --yes`  -  Add a client, or add domains and hosts to it; without `--yes` (CLI) or `confirm: true` (MCP) it only previews and exits 2.
- `domain-cert-tracker-cli clients remove <client> --domain d | --host h | --whole-client`  -  Remove the named entries, or the whole client with `--whole-client`; without `--yes` (CLI) or `confirm: true` (MCP) it only previews and exits 2.
- `domain-cert-tracker-cli clients import <file.csv|file.json>`  -  Import inventory from a file you name; command line only, not offered over MCP.


**dns**


- `domain-cert-tracker-cli dns check <domain>`  -  Get NS, MX, SPF, DMARC, CAA and DNSSEC for a domain as one JSON document through the system resolver.


**events**


- `domain-cert-tracker-cli events list`  -  Read typed events from each sweep diff (domain.expiring, cert.expiring, ct.new_cert, dns.changed, registrar.unlocked) after a cursor.


**hosts**


- `domain-cert-tracker-cli hosts discover`  -  List hostnames that have certificates in CT for a client's domains but are not in that client's inventory.


**issuers**


- `domain-cert-tracker-cli issuers audit`  -  Separate certificates from issuers a domain's CAA does not permit, or a first-ever issuer, from routine renewals.


**renewals**


- `domain-cert-tracker-cli renewals`  -  Find hosts still serving an old certificate after a newer one was logged, or past their renewal point with no successor in CT.


**report**


- `domain-cert-tracker-cli report`  -  See, per client, what expires within N days, which registrations are not transfer-locked, new CT certificates and DNS drift, plus what the last sweep could not check.


**sweep**


- `domain-cert-tracker-cli sweep`  -  Check every inventoried domain's registration, new CT certificates and DNS, and every inventoried host's live TLS certificate, in one run; then store the results and diff them against the last run.


**tls**


- `domain-cert-tracker-cli tls probe <host[:port]>`  -  Handshake with a live host and get its chain, expiry, SANs, key, issuer, hostname match and OCSP must-staple as JSON.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
domain-cert-tracker-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Narrow an RDAP answer to what matters

```bash
domain-cert-tracker-cli domain example.com --agent --select ldhName,status,events.eventAction,events.eventDate
```

RDAP domain objects carry long notices and vCards; --select keeps the status list and the registration and expiration dates.

### Monday estate check

```bash
domain-cert-tracker-cli report --days 30 --agent
```

Per client: what expires in 30 days, unlocked registrations, new CT certificates, DNS drift, and what could not be checked.

### Hand changes to a ticketing agent

```bash
domain-cert-tracker-cli events list --since 0 --agent
```

Returns typed events after the cursor plus the next cursor to pass next time.

### Find renewals that never reached the server

```bash
domain-cert-tracker-cli renewals --agent
```

Every inventoried host with a status; deploy_stalled rows (a newer certificate from the same CA logged but not served) come first.

### Check one client's renewals

```bash
domain-cert-tracker-cli renewals --client "Acme Dental" --agent
```

Only that client's hosts: stalled deployments first, then hosts whose renewal is overdue.

## Auth Setup

No credentials. RDAP (through rdap.org and the IANA bootstrap file) and crt.sh are public. HTTP lookups and TLS probes connect only to public addresses, over https or TLS on port 443 unless the command line passes --allow-port; DNS checks use the resolver your operating system is configured with.

Run `domain-cert-tracker-cli doctor` to verify setup.

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
  domain-cert-tracker-cli autnum 15169 --agent --select country,endAutnum,entities
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  report, renewals, issuers audit, hosts discover and events list read the local store that sweep fills
- **Non-interactive**  -  never prompts, every input is a flag
- **Read-only toward remote services**  -  do not use this CLI to create, update, delete, renew, transfer or send anything; it writes only its local inventory and store

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

- Use `--home <dir>` for one invocation, or set `DOMAIN_CERT_TRACKER_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `DOMAIN_CERT_TRACKER_CONFIG_DIR`, `DOMAIN_CERT_TRACKER_DATA_DIR`, `DOMAIN_CERT_TRACKER_STATE_DIR`, `DOMAIN_CERT_TRACKER_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `DOMAIN_CERT_TRACKER_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains durable local data such as `data.db`. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Run `domain-cert-tracker-cli doctor --fail-on warn` to surface path warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "domain-cert-tracker": {
        "command": "domain-cert-tracker-mcp",
        "env": {
          "DOMAIN_CERT_TRACKER_HOME": "/srv/domain-cert-tracker"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `DOMAIN_CERT_TRACKER_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `DOMAIN_CERT_TRACKER_HOME`, or `doctor` will not find files left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Sweep results and the client inventory are in the SQLite store; a response with an id or name the store can key it by (`ip`, `autnum`) is stored whole, `domain` and `nameserver` lookups are not, and `--data-source local` finds a saved lookup only by that stored key. GET responses are cached for five minutes under the cache dir. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `DOMAIN_CERT_TRACKER_NO_LOCAL_DATA=1` disables the response cache and writes of API data, turns `auto` reads into live reads, and blocks `sync`, `sweep` and `workflow archive` (the client inventory commands still work); `--data-source local` still reads an existing store, and `DOMAIN_CERT_TRACKER_DATA_SOURCE` sets the `--data-source` default.

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
domain-cert-tracker-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "domain-cert-tracker-cli learnings confirm 12"] }
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
       materially more, record the divergence via `domain-cert-tracker-cli playbook amend --query "$QUERY" --add-note <note>`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `domain-cert-tracker-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "LE" teach satisfying a "Let's Encrypt" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "DigiCert" → two different DigiCert issuing CAs). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `domain-cert-tracker-cli sweep` to refresh the store.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
domain-cert-tracker-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
domain-cert-tracker-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --resource-type <type> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
domain-cert-tracker-cli teach-playbook \
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
domain-cert-tracker-cli playbook amend \
  --query "$QUERY" \
  --add-note "$NOTE"
# (append shell `&` to background it)
```

What counts as worth amending: a behavior you OBSERVED this session that future-you would benefit from knowing. Examples worth amending:

- A workaround for a CLI surface that silently drops or misorders a flag.
- An undocumented endpoint shape (response wrapped in `{meta, results}`, payload nested two levels deeper than the docs claim).
- Observed schema drift (a field renamed, an index that shifted between registry releases, a category label that the API now returns lower-cased).

What does NOT belong in notes:

- The year-specific or entity-specific answer to the user's question. That's the response, not a learning.
- Per-client / per-domain / per-row data the playbook already retrieves at runtime.
- Statements that paraphrase what the existing notes already say.

The amend command appends to the family's existing notes with a timestamped marker (`[amend YYYY-MM-DDTHH:MMZ]: <text>`). Multiple amends accumulate; the audit trail is visible. If no playbook exists yet for the family, amend creates a notes-only one (so cold-start corrections still land).

#### PII discipline for amend notes

`playbook amend` notes are designed to potentially flow upstream as shared knowledge in future versions of the Printing Press. Keep them clean of user-identifying content so the upstream-contribution path stays open without retroactive scrubbing:

- **Do NOT embed** paths to user filesystems, personal API keys or tokens, user email addresses, user GitHub handles, or specific query histories tied to a single user.
- **Acceptable**: endpoint shapes, undocumented field names, API gotchas, observed schema drift, workarounds for CLI surfaces, generalizable pagination or retry tactics.

If a correction is only meaningful with user-specific context, it belongs in a personal note, not in the playbook amend.

### Measuring the loop

`domain-cert-tracker-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `DOMAIN_CERT_TRACKER_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
domain-cert-tracker-cli feedback "the --since flag is inclusive but docs say exclusive"
domain-cert-tracker-cli feedback --stdin < notes.txt
domain-cert-tracker-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `DOMAIN_CERT_TRACKER_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `DOMAIN_CERT_TRACKER_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only. Messages show only the endpoint's scheme and host.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` as a private 0600 file (exclusive randomly named temp in the destination directory, then rename; a planted symlink is not followed). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`). Redirects are not followed; a 3xx response fails the delivery and the body is not sent to the `Location`. |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures, including redirects, return non-zero and log the webhook's scheme and host and the numeric HTTP status on stderr; its user, path and query, and what the server answered, are never shown. `profile save` stores a `--deliver` URL as given, so keep a token out of a URL you save in a profile.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
domain-cert-tracker-cli profile save briefing --json
domain-cert-tracker-cli --profile briefing autnum 15169
domain-cert-tracker-cli profile list --json
domain-cert-tracker-cli profile show briefing
domain-cert-tracker-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | A check or command failed (every sweep check failed, a DNS lookup failed, a TLS handshake failed) |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 5 | API error (upstream issue) |
| 7 | Rate limited or blocked by a firewall or bot challenge (back off; do not retry immediately) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `domain-cert-tracker-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add domain-cert-tracker-mcp -- domain-cert-tracker-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `domain-cert-tracker-mcp --transport http` requires `DOMAIN_CERT_TRACKER_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `DOMAIN_CERT_TRACKER_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `DOMAIN_CERT_TRACKER_MCP_READ_ONLY=1` exposes only read tools: it drops `sweep`, `sync`, `export`, `clients set`, `clients remove`, the learn-loop writers (`teach`, `teach-pattern`, `teach-lookup`, `learnings confirm`, `learnings reject`, `learnings forget`, `playbook amend`) and `workflow archive`.

## Direct Use

1. Check if installed: `which domain-cert-tracker-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   domain-cert-tracker-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `domain-cert-tracker-cli <command> --help`.
