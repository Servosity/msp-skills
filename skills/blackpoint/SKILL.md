---
name: blackpoint
description: "CompassOne from the terminal: tenants, detections, assets, vulnerabilities, posture, reports and Cloud Response admin, plus a cross-tenant fleet view of open detections, stale agents and coverage gaps. Trigger phrases: `open blackpoint detections`, `compassone stale agents`, `which customers have open detections`, `blackpoint posture score`, `use blackpoint`, `run blackpoint-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Blackpoint Cyber"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - blackpoint-cli
    install:
      - kind: script
        bins: [blackpoint-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blackpoint/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blackpoint/install.ps1
---

# Blackpoint Cyber CompassOne - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `blackpoint-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blackpoint/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blackpoint/install.ps1 | iex
   ```
3. Verify: `blackpoint-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `blackpoint-cli` and `blackpoint-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Commands for the CompassOne endpoints documented in Blackpoint's public SDK and the community MCP servers, with --dry-run on every write. fleet-sync pulls every tenant into a local store so triage, gaps, stale-agents, cve-spread and leaderboard answer cross-customer questions in one command, offline.

## When to Use This CLI

Use this CLI when an agent or MSP engineer needs Blackpoint CompassOne data: detections, assets, vulnerabilities, posture and reports for a customer, or a cross-tenant view across every customer. It also administers contact groups and Cloud Response approved countries with dry-run on every write.

## Anti-triggers

Do not use this CLI for:
- Do not use it to resolve or claim SOC tickets; the API exposes detections read-only.
- Do not use it to install or deploy the SNAP agent on endpoints; use your RMM.
- Do not use it for Blackpoint billing or contract changes; use the partner portal.

## Unique Capabilities

### Cross-tenant fleet view
- **`triage`**  -  One oldest-first queue of open detections across every customer tenant, with tenant name, age, alert count and ticket status. Reads the local store, so run fleet-sync first; before that it returns an empty list with a stderr hint.

  _Reach for this for the morning SOC sweep instead of calling detections list once per tenant._

  ```bash
  blackpoint-cli triage --older-than 24h --type MDR --agent
  ```
- **`gaps`**  -  Find tenants with no recent completed scan, no or empty escalation contact group, or no posture rating; scans_unknown, contacts_unknown or posture_unknown means that data could not be fetched. Reads the local store, so run fleet-sync first; before that it returns an empty list with a stderr hint.

  _Use it to answer which customers are unprotected or unreachable before an incident proves it._

  ```bash
  blackpoint-cli gaps --scan-days 30 --agent
  ```
- **`stale-agents`**  -  List devices across every tenant whose agent has not reported in N days or never checked in, with Defender status; decommissioned devices are skipped unless --include-decommissioned. Reads the local store, so run fleet-sync first; before that it returns an empty list with a stderr hint.

  _Use it for weekly endpoint hygiene across the whole book of customers._

  ```bash
  blackpoint-cli stale-agents --days 14 --agent
  ```
- **`cve-spread`**  -  Rank unresolved CVEs by how many tenants and assets they affect; --exploited or --prioritized narrows the list. Needs the vulnerability-management entitlement: when every tenant's fetch was refused (403) it fails instead of returning an empty list. Reads the local store, so run fleet-sync first; before that it returns an empty list with a stderr hint.

  _Use it to decide which patch campaign protects the most customers first._

  ```bash
  blackpoint-cli cve-spread --exploited --agent
  ```
- **`fleet-sync`**  -  Pull every tenant's detections, devices, vulnerabilities, scans, exposures, posture, contact groups and approved countries into the local store; failed or incomplete fetches are kept, reported under fetch_failures, and flagged by the readers.

  _Run it before triage, gaps, stale-agents, cve-spread, overview, leaderboard or geo-audit so they read fresh data._

  ```bash
  blackpoint-cli fleet-sync --agent
  ```

### Client reporting
- **`client-report`**  -  Live: build one client's reporting-window summary from six CompassOne endpoints (posture, detection count and weekly trend, top entities and threats, latest report run per type). Needs BLACKPOINT_API_TOKEN; failed sections are listed under fetch_failures.

  _Use it when an agent drafts a monthly client security report; one call replaces six._

  ```bash
  blackpoint-cli client-report "Acme Dental" --since 30d --agent
  ```
- **`overview`**  -  One offline current-state view of a tenant: open detections, stale devices, exploited or prioritized CVEs, last scans, posture, dark-web exposure counts, cloud connection state and escalation contacts. Reads the local store, so run fleet-sync first; an unknown tenant or a store with no fleet data exits 3 (not found).

  _Use it before a client call or escalation to see everything about one customer at once._

  ```bash
  blackpoint-cli overview "Acme Dental" --agent
  ```
- **`leaderboard`**  -  Rank tenants by security posture score as a share of their maximum, with maturity level and weakest NIST function. Reads the local store, so run fleet-sync first; before that it returns an empty list with a stderr hint.

  _Use it to pick which clients need attention this week._

  ```bash
  blackpoint-cli leaderboard --agent
  ```

### Cloud Response admin
- **`geo-audit`**  -  Audit Cloud Response approved countries per connection and per user; flag expired or expiring travel exceptions and broad allow-lists. Reads the local store, so run fleet-sync first; before that it returns an empty list with a stderr hint.

  _Use it weekly so temporary travel exceptions do not silently become permanent._

  ```bash
  blackpoint-cli geo-audit --expiring 7d --agent
  ```

## Command Reference

**accounts**  -  Partner accounts this token can see.

- `blackpoint-cli accounts get`  -  Returns one partner account.
- `blackpoint-cli accounts list`  -  Lists the partner accounts this token can see. Wire: GET /v1/accounts, returns {data, meta}.

**assets**  -  Per-tenant asset inventory by class, and the relationship graph.

- `blackpoint-cli assets get`  -  Returns one asset; fields depend on its class.
- `blackpoint-cli assets list`  -  Lists assets for one tenant. `class` is required: there is no all-assets query.
- `blackpoint-cli assets list-relationships`  -  Lists what one asset is connected to in one direction.

**cisco-onboardings**  -  Cisco Duo Cloud Response onboarding.

- `blackpoint-cli cisco-onboardings complete`  -  Completes the onboarding by verifying everything was set up appropriately.
- `blackpoint-cli cisco-onboardings create`  -  Begins a new onboarding for a customer. Fails if the onboarding for that domain already exists in Blackpoint.
- `blackpoint-cli cisco-onboardings delete`  -  Abandons an onboarding, removing the db records and cisco duo onboarding app if there is any.
- `blackpoint-cli cisco-onboardings get`  -  Get the state of an onboarding.
- `blackpoint-cli cisco-onboardings list`  -  List the states of the onboardings a customer has completed or in progress.
- `blackpoint-cli cisco-onboardings sync-users`  -  Runs the Duo post-provision step

**cloud-connections**  -  Cloud Response connection approved countries and per-user travel exceptions.

- `blackpoint-cli cloud-connections approve-country`  -  Approve a country for a given connection. Body: {isoCountryCode}.
- `blackpoint-cli cloud-connections approve-user-country`  -  Add approved countries for connection user. Body: {isoCountryCode, startDate?, endDate?} (a travel exception window).
- `blackpoint-cli cloud-connections list-approved-countries`  -  Get a paginated list of approved countries for a given connection.
- `blackpoint-cli cloud-connections list-user-countries`  -  List a user's approved countries
- `blackpoint-cli cloud-connections list-users`  -  Get users for connection.
- `blackpoint-cli cloud-connections remove-approved-country`  -  Remove a country for a given connection by its ID.
- `blackpoint-cli cloud-connections remove-user-country`  -  Delete connection user approved country.

**contact-groups**  -  Escalation contact groups, their members and tenant assignment.

- `blackpoint-cli contact-groups add-member`  -  Add member to contact group.
- `blackpoint-cli contact-groups assign-tenants`  -  Assign tenants to a contact group
- `blackpoint-cli contact-groups create`  -  Body: TEV1CreateContactGroupRequestDto.
- `blackpoint-cli contact-groups delete`  -  Delete contact group.
- `blackpoint-cli contact-groups delete-many`  -  Bulk delete: DELETE with a JSON body {contactGroupIds: [...]}.
- `blackpoint-cli contact-groups delete-member`  -  Delete contact group member.
- `blackpoint-cli contact-groups get`  -  Get contact group.
- `blackpoint-cli contact-groups get-member`  -  Get contact group member.
- `blackpoint-cli contact-groups list`  -  Escalation contact groups for an account.
- `blackpoint-cli contact-groups list-members`  -  List contact group members.
- `blackpoint-cli contact-groups list-tenants`  -  List tenants assigned to a contact group.
- `blackpoint-cli contact-groups list-unassigned-tenants`  -  Get a paginated list of all tenants which have not being assigned to a given contact group.
- `blackpoint-cli contact-groups update`  -  Body: TEV1UpdateContactGroupRequestDto; members carry their id to keep them.
- `blackpoint-cli contact-groups update-member`  -  Body: TEV1UpdateContactGroupMemberRequestDto (all fields required).

**countries**  -  ISO country reference list for Cloud Response geo policy.

- `blackpoint-cli countries get`  -  Get country by code.
- `blackpoint-cli countries list`  -  List countries. Reference list used by the approved-country (geo) policy.

**darkweb-exposures**  -  Dark-web credential exposures from a tenant's latest scan.

- `blackpoint-cli darkweb-exposures`  -  Credential exposures from the tenant's most recent dark-web scan.

**detections**  -  Detections (alert groups) per tenant, with counts, trends and top entities and threats.

- `blackpoint-cli detections count`  -  Counts detections without listing them. Response shape is not published; returned verbatim.
- `blackpoint-cli detections get`  -  Returns one detection in full.
- `blackpoint-cli detections list`  -  Lists detections for one tenant, paged with skip/take.
- `blackpoint-cli detections list-alerts`  -  Lists the individual alerts a detection aggregated.
- `blackpoint-cli detections list-by-week`  -  Detection counts bucketed by week (the trend series behind a monthly report chart). Response returned verbatim.
- `blackpoint-cli detections list-top-by-entity`  -  Ranks the hosts and users with the most detections. Response returned verbatim.
- `blackpoint-cli detections list-top-by-threat`  -  Ranks the threat types behind the most detections. Response returned verbatim.

**external-exposures**  -  Findings from one external (perimeter) scan.

- `blackpoint-cli external-exposures <scanId>`  -  Findings from one external (perimeter) scan. Pick the scan with `scans --type external`. Response returned verbatim.

**google-onboardings**  -  Google Workspace Cloud Response onboarding state.

- `blackpoint-cli google-onboardings get`  -  Get the state of an onboarding.
- `blackpoint-cli google-onboardings list`  -  List the states of the onboardings a customer has completed or in progress.

**ms365**  -  Microsoft 365 Cloud Response connections, users and approved countries.

- `blackpoint-cli ms365 approve-country`  -  Post iso country to ms365 connection authorized country list. Body: {code}.
- `blackpoint-cli ms365 approve-user-country`  -  Adds an authorized country to user. Body: {isoCountryCode, startDate?, endDate?}.
- `blackpoint-cli ms365 get-connection`  -  Get cloud ms365 connections by ID. Includes onboarding stage, policy config, CIS benchmarks and events.
- `blackpoint-cli ms365 list-approved-countries`  -  List m365 connection approved countries. Returns the connection with authorizedCountries.
- `blackpoint-cli ms365 list-connections`  -  Get cloud ms365 customer. Returns the tenant with its ms365DefensePackages (one per connected Microsoft 365 tenant).
- `blackpoint-cli ms365 list-user-countries`  -  Get active isoCountries for a given user.
- `blackpoint-cli ms365 list-users`  -  Get ms ms365 defense connection's users.
- `blackpoint-cli ms365 remove-country`  -  Delete iso country from ms365 connection authorized country list. The country is a `code` query parameter.
- `blackpoint-cli ms365 remove-user-country`  -  Deletes an authorized country from user.

**posture**  -  Security posture rating and metric breakdown for one tenant.

- `blackpoint-cli posture`  -  Score, maximumScore, maturityLevel and every metric row.

**reports**  -  Generated report runs and their JSON payloads.

- `blackpoint-cli reports get-json`  -  The report payload, verbatim.
- `blackpoint-cli reports list`  -  Generated report runs for one tenant. startDate/endDate filter on intervalStart (inclusive).

**scans**  -  Vulnerability scan runs for one tenant.

- `blackpoint-cli scans`  -  Scan runs and their status. A tenant with no recent completed scan has no coverage, not a clean bill of health.

**tenants**  -  Customer tenants; the source of every tenant id.

- `blackpoint-cli tenants get`  -  Returns one tenant with its status, contact group (and members) and report/alert email lists.
- `blackpoint-cli tenants list`  -  Lists the customer tenants this token can reach.

**users**  -  Portal users visible to this token.

- `blackpoint-cli users`  -  Portal users visible to this token. Response returned verbatim; 403 means the key is not entitled.

**vulnerabilities**  -  Vulnerability findings for one tenant.

- `blackpoint-cli vulnerabilities`  -  CVE-level findings for one tenant with severity, scores and affected-asset counts.


**client-report**


- `blackpoint-cli client-report`  -  Live: build one client's reporting-window summary from six CompassOne endpoints (posture, detection count and weekly trend, top entities and threats, latest report run per type). Needs BLACKPOINT_API_TOKEN; failed sections are listed under fetch_failures.


**cve-spread**


- `blackpoint-cli cve-spread`  -  Rank unresolved CVEs by how many tenants and assets they affect; --exploited or --prioritized narrows the list. Needs the vulnerability-management entitlement: when every tenant's fetch was refused (403) it fails instead of returning an empty list. Reads the local store, so run fleet-sync first; before that it returns an empty list with a stderr hint.


**fleet-sync**


- `blackpoint-cli fleet-sync`  -  Pull every tenant's detections, devices, vulnerabilities, scans, exposures, posture, contact groups and approved countries into the local store; failed or incomplete fetches are kept, reported under fetch_failures, and flagged by the readers.


**gaps**


- `blackpoint-cli gaps`  -  Find tenants with no recent completed scan, no or empty escalation contact group, or no posture rating; scans_unknown, contacts_unknown or posture_unknown means that data could not be fetched. Reads the local store, so run fleet-sync first; before that it returns an empty list with a stderr hint.


**geo-audit**


- `blackpoint-cli geo-audit`  -  Audit Cloud Response approved countries per connection and per user; flag expired or expiring travel exceptions and broad allow-lists. Reads the local store, so run fleet-sync first; before that it returns an empty list with a stderr hint.


**leaderboard**


- `blackpoint-cli leaderboard`  -  Rank tenants by security posture score as a share of their maximum, with maturity level and weakest NIST function. Reads the local store, so run fleet-sync first; before that it returns an empty list with a stderr hint.


**overview**


- `blackpoint-cli overview`  -  One offline current-state view of a tenant: open detections, stale devices, exploited or prioritized CVEs, last scans, posture, dark-web exposure counts, cloud connection state and escalation contacts. Reads the local store, so run fleet-sync first; an unknown tenant or a store with no fleet data exits 3 (not found).


**stale-agents**


- `blackpoint-cli stale-agents`  -  List devices across every tenant whose agent has not reported in N days or never checked in, with Defender status; decommissioned devices are skipped unless --include-decommissioned. Reads the local store, so run fleet-sync first; before that it returns an empty list with a stderr hint.


**triage**


- `blackpoint-cli triage`  -  One oldest-first queue of open detections across every customer tenant, with tenant name, age, alert count and ticket status. Reads the local store, so run fleet-sync first; before that it returns an empty list with a stderr hint.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
blackpoint-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Morning sweep

```bash
blackpoint-cli triage --older-than 24h --agent
```

Run fleet-sync first; then this lists open detections across all tenants, oldest first.

### Narrow a big detection payload

```bash
blackpoint-cli detections list --x-tenant-id 3f2a9c1e-7b4d-4e8a-9f10-2c6d8b5e4a71 --agent --select id,hostname,alertCount,ticket.status
```

Keep only the fields an agent needs from a nested alert-group page.

### Monthly client report

```bash
blackpoint-cli client-report "Acme Dental" --since 30d --agent
```

Live (needs BLACKPOINT_API_TOKEN): posture, detection counts, weekly trend, top entities and threats, and the latest report run per report type in the window.

### Who is uncovered

```bash
blackpoint-cli gaps --scan-days 30 --agent
```

Run fleet-sync first. Gap codes: no_recent_scan, no_contact_group, empty_contact_group, no_posture_rating; scans_unknown, contacts_unknown or posture_unknown means that data could not be fetched, not that coverage is missing.

## Auth Setup

Create an API token in the CompassOne portal and export BLACKPOINT_API_TOKEN. A partner token sees every tenant; a tenant token sees one customer. Tenant-scoped commands (detections, assets, vulnerabilities, scans, posture, reports) take --x-tenant-id <tenant uuid>, which you get from 'tenants list'. Cloud Response commands take --tenant-id instead.

Run `blackpoint-cli doctor` to verify setup.

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
  blackpoint-cli accounts list --agent --select billingVersion,created,id
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

- Use `--home <dir>` for one invocation, or set `BLACKPOINT_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `BLACKPOINT_CONFIG_DIR`, `BLACKPOINT_DATA_DIR`, `BLACKPOINT_STATE_DIR`, `BLACKPOINT_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `BLACKPOINT_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `blackpoint-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "blackpoint": {
        "command": "blackpoint-mcp",
        "env": {
          "BLACKPOINT_HOME": "/srv/blackpoint"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `BLACKPOINT_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `BLACKPOINT_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are cached for five minutes under the cache dir. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `BLACKPOINT_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `BLACKPOINT_DATA_SOURCE` sets the `--data-source` default.

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
blackpoint-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "blackpoint-cli learnings confirm 12"] }
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
       materially more, record the divergence via `blackpoint-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `blackpoint-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `blackpoint-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
blackpoint-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
blackpoint-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --resource-type <type> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
blackpoint-cli teach-playbook \
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
blackpoint-cli playbook amend \
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

`blackpoint-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `BLACKPOINT_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
blackpoint-cli feedback "the --since flag is inclusive but docs say exclusive"
blackpoint-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `BLACKPOINT_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `BLACKPOINT_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
blackpoint-cli profile save briefing --json
blackpoint-cli --profile briefing accounts list
blackpoint-cli profile list --json
blackpoint-cli profile show briefing
blackpoint-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `blackpoint-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add blackpoint-mcp -e BLACKPOINT_API_TOKEN=<your-token> -- blackpoint-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `blackpoint-mcp --transport http` requires `BLACKPOINT_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `BLACKPOINT_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `BLACKPOINT_MCP_READ_ONLY=1` lists only read-only tools: it removes `blackpoint_execute`, `sync`, `fleet_sync`, `import`, `export`, `workflow_archive` and the learn-store writers (`teach`, `teach_lookup`, `teach_pattern`, `teach_playbook`, `playbook_amend`, `learnings_confirm`, `learnings_reject`, `learnings_forget`), 14 of the 36 tools, and `blackpoint_read` still refuses every state-changing endpoint.

## Direct Use

1. Check if installed: `which blackpoint-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   blackpoint-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `blackpoint-cli <command> --help`.
