---
name: abnormal
description: "The full Abnormal Security REST API as an agent-ready CLI  -  with a local threat store, ranked SOC triage, and one-shot reporting no SOAR pack offers. Trigger phrases: `triage abnormal threats`, `remediate a phishing campaign in abnormal`, `abnormal email threat report`, `check account takeover risk for an employee`, `vendor email compromise check`, `use abnormal security`, `run abnormal-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Abnormal Security"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - abnormal-cli
    install:
      - kind: script
        bins: [abnormal-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/abnormal/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/abnormal/install.ps1
---

# Abnormal Security - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `abnormal-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/abnormal/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/abnormal/install.ps1 | iex
   ```
3. Verify: `abnormal-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `abnormal-cli` and `abnormal-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Every threat, case, vendor, employee, and dashboard operation from Abnormal's official API, plus a synced SQLite store that powers a ranked `triage` queue, joined `employee-risk` and `vendor-risk` investigation views, blocking `remediate-watch` confirmation, and a consolidated `report-snapshot` for client reporting. The incumbent integrations live inside SOAR and SIEM platforms; this runs in your terminal and your agents.

## When to Use This CLI

Use this CLI for SOC work against an Abnormal Security tenant: triaging the email threat feed, confirming remediations, investigating account-takeover cases and vendor email compromise, pulling employee identity and login context, and producing security reports from the dashboard aggregations. It is the right choice whenever an agent needs Abnormal threat or case data in structured form, or needs to act on a threat and verify the action completed.

## Anti-triggers

Do not use this CLI for:
- Quarantining or purging mail directly inside Microsoft 365 mailboxes  -  Abnormal remediates through its own actions; use Graph or Defender tooling for raw mailbox surgery
- Sending or composing email
- Changing Abnormal detection policies or integration settings  -  those are portal-only and not exposed by the REST API
- Continuous high-volume SIEM ingestion  -  use Abnormal's native Splunk/Elastic integrations for streaming pipelines

## Unique Capabilities

### SOC triage that compounds
- **`triage`**  -  Surface the newest, highest-severity, still-unremediated threats first so an analyst starts the shift on what actually matters.

  _Reach for this when asked what email threats need attention now  -  it returns a ranked queue instead of a flat list._

  ```bash
  abnormal-cli triage --since 24h --top 20 --agent
  ```
- **`remediate-watch`**  -  Remediate a threat or case and block until Abnormal reports the action reached a terminal state, with a typed exit code and receipt.

  _Use this when a remediation must be confirmed done, not just submitted  -  exit code 0 means the action completed._

  ```bash
  abnormal-cli remediate-watch threat 184712ab-6d8b-47b3-89d7-a314efef23ff --timeout 5m
  ```

### Reporting and onboarding
- **`report-snapshot`**  -  Pull the key dashboard aggregations  -  attacks seen, attacks stopped, impersonation breakdowns, trending attacks  -  into one client-ready table or CSV.

  _Use this to build a weekly or QBR security report without screenshotting dashboard panels._

  ```bash
  abnormal-cli report-snapshot --since 30d --csv
  ```
- **`smoke`**  -  Verify token, base URL (US/EU), and IP allowlist reachability using Abnormal's vendor-supplied test payloads  -  without touching real tenant data.

  _Run this first when onboarding a tenant or rotating a token to prove connectivity before real queries._

  ```bash
  abnormal-cli smoke
  ```

### Investigation joins
- **`employee-risk`**  -  One account-takeover risk picture per employee: profile, Genome identity analysis, 30-day login pattern, and open cases naming them.

  _Use this when investigating a suspected account takeover to scope identity and blast radius in one call._

  ```bash
  abnormal-cli employee-risk vip@example.com --agent
  ```
- **`vendor-risk`**  -  One vendor-email-compromise picture per vendor: details, recent activity, and open vendor cases.

  _Use this when a vendor's emails look suspicious to see their whole relationship and case history at once._

  ```bash
  abnormal-cli vendor-risk supplier.example.com --agent
  ```

## Command Reference

**abuse-mailbox**  -  Manage abuse mailbox

- `abnormal-cli abuse-mailbox`  -  Get a list of messages submitted to AI Security Mailbox (formerly known as Abuse Mailbox) that were not analyzed.

**abusecampaigns**  -  Manage abusecampaigns

- `abnormal-cli abusecampaigns retrieve`  -  Get a list of campaigns submitted to AI Security Mailbox (formerly known as Abuse Mailbox)
- `abnormal-cli abusecampaigns retrieve-2`  -  Get details of an abuse campaign

**aggregations**  -  Manage aggregations

- `abnormal-cli aggregations attack-frequency-retrieve`  -  Retrieve the frequency of specific attack types for a given period.
- `abnormal-cli aggregations attack-stopped-retrieve`  -  Retrieve aggregated counts of distinct attack types that were successfully stopped
- `abnormal-cli aggregations attack-strategy-breakdown-retrieve`  -  Retrieve the breakdown of attacks based on their strategy.
- `abnormal-cli aggregations attack-vector-breakdown-retrieve`  -  Retrieve the breakdown of attacks based on their vectors.
- `abnormal-cli aggregations attacker-origin-retrieve`  -  Retrieve the origin countries of attackers for a given period.
- `abnormal-cli aggregations dashboard-summary-retrieve`  -  Retrieve an aggregated summary of multiple security data points for the dashboard.
- `abnormal-cli aggregations most-impersonated-employee-non-vip-retrieve`  -  Retrieve the most impersonated non-VIP employees for a specified period.
- `abnormal-cli aggregations most-impersonated-employee-retrieve`  -  Retrieve the most impersonated employees for a specified period.
- `abnormal-cli aggregations most-impersonated-employee-vip-retrieve`  -  Retrieve the most impersonated VIP employees for a specified period.
- `abnormal-cli aggregations most-impersonated-vendor-retrieve`  -  Retrieve a list of the most impersonated vendors in attacks.
- `abnormal-cli aggregations recipient-employees-non-vip-retrieve`  -  Retrieve a list of the non-VIP employees who were recipients of attacks, based on their job titles.
- `abnormal-cli aggregations recipient-employees-retrieve`  -  Retrieve a list of the employees who were recipients of attacks, based on their job titles.
- `abnormal-cli aggregations recipient-employees-vip-retrieve`  -  Retrieve a list of the VIP employees who were recipients of attacks, based on their job titles.
- `abnormal-cli aggregations sender-impersonation-breakdown-retrieve`  -  Retrieve a breakdown of attacks based on sender impersonation.
- `abnormal-cli aggregations trending-attacks-retrieve`  -  Retrieve the list of trending attacks for a specified period.

**api_resources**  -  Manage api resources

- `abnormal-cli api-resources resources-actions-create`  -  Execute a specific action on a resource (refresh or validate). Returns 202 Accepted with action ID for tracking.
- `abnormal-cli api-resources resources-create-create`  -  Create a new resource with the specified name and optional description. Returns 201 Created with resource ID.
- `abnormal-cli api-resources resources-retrieve`  -  Retrieve a paginated list of resources with optional filtering using pageSize and pageNumber query parameters.
- `abnormal-cli api-resources resources-retrieve-2`  -  Retrieve detailed information about a specific resource by its UUID.
- `abnormal-cli api-resources resources-update-partial-update`  -  Partially update an existing resource's fields (PATCH). Provide only the fields that need updating.

**auditlogs**  -  Manage auditlogs

- `abnormal-cli auditlogs`  -  Gets a list of Audit Logs for Portal

**cases**  -  APIs to manage Abnormal Cases

- `abnormal-cli cases create`  -  Account Takeover license is required to call this endpoint. Use this to update the status of an abnormal case.
- `abnormal-cli cases retrieve`  -  Get a list of Abnormal cases identified by Abnormal Security
- `abnormal-cli cases retrieve-2`  -  Account Takeover license is required to call this endpoint.

**detection360**  -  Manage detection360

- `abnormal-cli detection360 reports-create`  -  Use this to report a detection misclassification judgement by Abnormal Security.
- `abnormal-cli detection360 reports-retrieve`  -  Get a list of Detection 360 reports that you have submitted and view corresponding details for each case

**email_search**  -  Manage email search

- `abnormal-cli email-search search-activities-retrieve`  -  List activity logs for search and remediation operations. Optionally filter by tenant_ids query parameter (e.g., ?
- `abnormal-cli email-search search-activities-status-retrieve`  -  Get detailed status of a specific activity including remediation results.
- `abnormal-cli email-search search-create`  -  Search for email messages across Abnormal and Quarantine sources. Optionally filter by tenant_ids in the request body.
- `abnormal-cli email-search search-messages-attachments-download-retrieve`  -  Download an email attachment for a given message.
- `abnormal-cli email-search search-messages-eml-retrieve`  -  Download the EML file for a specific message by cloud_message_id. Returns the EML file content as message/rfc822 format.
- `abnormal-cli email-search search-remediate-create`  -  Remediate email messages by deleting, moving, or submitting them for review.

**employee**  -  Manage employee

- `abnormal-cli employee <email_address>`  -  Get employee information

**messages**  -  API to manage message details


**roles**  -  API to retrieve roles from RBAC system

- `abnormal-cli roles`  -  Fetch all roles for an account from RBAC system.

**security-settings**  -  API to retrieve security settings including session timeout configuration

- `abnormal-cli security-settings`  -  Fetch security settings for an account.

**soar**  -  Manage soar

- `abnormal-cli soar`  -  Fetch all API tokens for the authenticated customer from the Go Token Management Service.

**spm-v2**  -  Manage spm v2

- `abnormal-cli spm-v2 posture-catalog-retrieve`  -  Get posture catalog containing all available abnormal supported postures
- `abnormal-cli spm-v2 postures-query-create`  -  Get a list of all tenant postures
- `abnormal-cli spm-v2 postures-retrieve`  -  Get detailed information about a specific security posture evaluation
- `abnormal-cli spm-v2 postures-timeline-retrieve`  -  Get timeline of events for a specific security posture
- `abnormal-cli spm-v2 reports-summary-retrieve`  -  Get summary report for all postures
- `abnormal-cli spm-v2 workflow-logs-raw-json-retrieve`  -  Get raw JSON for a workflow log

**threats**  -  APIs to manage threats notified in the Abnormal Threat Log

- `abnormal-cli threats create`  -  Use this to remediate or unremediate a threat.
- `abnormal-cli threats retrieve`  -  Get a list of threats
- `abnormal-cli threats retrieve-2`  -  Get details of a threat

**threats-export**  -  Manage threats export

- `abnormal-cli threats-export`  -  Download data from Threat Log in .csv format

**url-rewrite**  -  Manage url rewrite

- `abnormal-cli url-rewrite`  -  Retrieve paginated click and clickthrough events for URL rewrites.

**users**  -  API to retrieve users from RBAC system

- `abnormal-cli users`  -  Retrieves users for an account from the RBAC user management system.

**vendor-cases**  -  Manage vendor cases

- `abnormal-cli vendor-cases retrieve`  -  Get a list of vendor cases.
- `abnormal-cli vendor-cases retrieve-2`  -  Get details of a vendor case

**vendors**  -  API to manage Vendorbase and threats from Vendors

- `abnormal-cli vendors`  -  Get a list of vendors your organization has interacted with


**employee-risk**


- `abnormal-cli employee-risk`  -  One account-takeover risk picture per employee: profile, Genome identity analysis, 30-day login pattern, and open cases naming them.


**remediate-watch**


- `abnormal-cli remediate-watch`  -  Remediate a threat or case and block until Abnormal reports the action reached a terminal state, with a typed exit code and receipt.


**report-snapshot**


- `abnormal-cli report-snapshot`  -  Pull the key dashboard aggregations  -  attacks seen, attacks stopped, impersonation breakdowns, trending attacks  -  into one client-ready table or CSV.


**smoke**


- `abnormal-cli smoke`  -  Verify token, base URL (US/EU), and IP allowlist reachability using Abnormal's vendor-supplied test payloads  -  without touching real tenant data.


**triage**


- `abnormal-cli triage`  -  Surface the newest, highest-severity, still-unremediated threats first so an analyst starts the shift on what actually matters.


**vendor-risk**


- `abnormal-cli vendor-risk`  -  One vendor-email-compromise picture per vendor: details, recent activity, and open vendor cases.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
abnormal-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Morning triage

```bash
abnormal-cli triage --since 24h --top 20 --agent --select items.threatId,items.attackType,items.score
```

Ranked unremediated threats from the local store, narrowed to the three fields an agent needs to decide what to open first.

### Remediate and confirm

```bash
abnormal-cli remediate-watch threat 184712ab-6d8b-47b3-89d7-a314efef23ff --timeout 5m
```

Submits the remediation and blocks until Abnormal reports a terminal action state  -  exit 0 means confirmed done.

### Account-takeover scoping

```bash
abnormal-cli employee-risk vip@example.com --agent
```

Profile, Genome identity analysis, 30-day logins, and open cases for one employee in a single structured payload.

### Client report numbers

```bash
abnormal-cli report-snapshot --since 30d --csv
```

The dashboard aggregations consolidated into CSV for pasting straight into a client report.

### Vendor compromise check

```bash
abnormal-cli vendor-risk supplier.example.com --agent
```

Vendor details, recent activity, and open vendor cases joined into one investigation view.

## Auth Setup

Mint a REST API token in the Abnormal portal under Settings → Integrations → Abnormal REST API, then export it as `ABNORMAL_API_TOKEN`. Abnormal enforces IP allowlisting on the integration: add your egress IP in the same portal screen or every call returns 403. EU tenants must point the CLI at the EU host: `export ABNORMAL_BASE_URL=https://eu.rest.abnormalsecurity.com/v1`. You can also store the token by piping it to `abnormal-cli auth set-token` (it reads stdin, never an argument) and check it with the auth status command.

Run `abnormal-cli doctor` to verify setup.

## Credential safety

- The `Authorization: Bearer` token and every API request go only to the scheme, host and port of the configured base URL (`https://api.abnormalplatform.com/v1`, or `ABNORMAL_BASE_URL` for the EU host). A redirect to any other host is refused, so the token never reaches it.
- Nothing the CLI or the MCP server prints, writes to a file or to the local store, or sends to a host other than the configured one can carry `ABNORMAL_API_TOKEN`, the stored or config-file token, or `ABNORMAL_MCP_HTTP_TOKEN`, in any encoding. The shared credential kit withholds the whole message, file write, store row or request, and prints "[credkit: withheld from <where>: it carried a credential this program holds]" in its place. Responses are never cached on disk.
- A `--stdin` request body is held as a credential too, whole and line by line. A `--dry-run` preview of it is withheld, and so is an answer that repeats part of it (that write still happens). `feedback --stdin` is refused for the same reason: pass the note as an argument.
- Credentials are stored only in `credentials.toml` and `config.toml`, both written mode 0600; `auth set-token` reads the token from stdin, never from an argument.
- `--dry-run` sends nothing. Over MCP, a tool that changes something in Abnormal (`abnormal_execute`, `remediate_watch`, `import`) returns a preview and sends only with `confirm: true`, and a tool argument can never name a file to read or write.

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
  abnormal-cli abuse-mailbox --agent --select abx_message_id,not_analyzed_reason,recipient
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
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

- Use `--home <dir>` for one invocation, or set `ABNORMAL_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `ABNORMAL_CONFIG_DIR`, `ABNORMAL_DATA_DIR`, `ABNORMAL_STATE_DIR`, `ABNORMAL_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `ABNORMAL_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `abnormal-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "abnormal": {
        "command": "abnormal-mcp",
        "env": {
          "ABNORMAL_HOME": "/srv/abnormal"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `ABNORMAL_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `ABNORMAL_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are never cached on disk. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `ABNORMAL_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `ABNORMAL_DATA_SOURCE` sets the `--data-source` default.

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
abnormal-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "abnormal-cli learnings confirm 12"] }
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
       materially more, record the divergence via `abnormal-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `abnormal-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `abnormal-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
abnormal-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
abnormal-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
abnormal-cli teach-playbook \
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
abnormal-cli playbook amend \
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

`abnormal-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `ABNORMAL_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
abnormal-cli feedback "the --since flag is inclusive but docs say exclusive"
abnormal-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `ABNORMAL_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `ABNORMAL_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
abnormal-cli profile save briefing --json
abnormal-cli --profile briefing abuse-mailbox
abnormal-cli profile list --json
abnormal-cli profile show briefing
abnormal-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `abnormal-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add abnormal-mcp -- abnormal-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `abnormal-mcp --transport http` requires `ABNORMAL_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `ABNORMAL_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `ABNORMAL_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which abnormal-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   abnormal-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `abnormal-cli <command> --help`.
