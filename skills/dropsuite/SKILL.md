---
name: dropsuite
description: "Every Dropsuite (NinjaOne SaaS Backup) read, plus fleet-wide backup health, coverage gaps and billing reconciliation from one reseller credential. Trigger phrases: `check dropsuite backups`, `which clients have failing saas backups`, `unprotected mailboxes in ninjaone saas backup`, `reconcile dropsuite seats with billing`, `use dropsuite`, `run dropsuite`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Dropsuite (NinjaOne)"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - dropsuite-cli
    install:
      - kind: script
        bins: [dropsuite-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dropsuite/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dropsuite/install.ps1
---

# Dropsuite Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `dropsuite-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dropsuite/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dropsuite/install.ps1 | iex
   ```
3. Verify: `dropsuite-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `dropsuite-cli` and `dropsuite-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Dropsuite's portal and API work one client at a time, with a different token per organization. This CLI resolves every organization's token from your reseller credential, so health, coverage, billing and hygiene answer for the whole fleet in one command. Health snapshots and a restore-test ledger build a dated evidence trail the API does not keep. Built from Dropsuite's public API reference and tested against a local mock; it has not yet been confirmed against a live partner tenant.

## When to Use This CLI

Use this CLI when an MSP needs answers about Dropsuite / NinjaOne SaaS Backup across many client organizations: failing or stale backups, unprotected mailboxes, seat and billing reconciliation, lifecycle cleanup, or audit evidence. It reads only (the sign-in link commands mint a portal URL) and works with reseller and sub-partner credentials. It was built from the vendor's public API reference and tested against a mock, not yet against a live partner tenant.

## Anti-triggers

Do not use this CLI for:
- Performing a restore: the API has no restore endpoint; get a one-time portal sign-in URL with portal get-login-link --org <id> and restore in the portal, then log it with restore-test record
- Adding or removing mailboxes from backup or changing subscriptions: the partner API for this tier is GET-only
- NinjaOne RMM devices, policies or tickets: that is the separate NinjaOne public API, not SaaS Backup

## Unique Capabilities

### Fleet-wide answers from one credential
- **`health`**  -  See every failed, stuck or stale mailbox, OneDrive, SharePoint site, Team, shared drive and Google Drive across your active clients in one run.

  _Reach for this for the morning backup sweep or any 'is anything failing across my clients' question; add --exit-code for Nagios-style exit codes._

  ```bash
  dropsuite-cli health --stale-after 48h --failing --agent
  ```
- **`coverage`**  -  List mailboxes that exist in each client's Microsoft 365 or Google Workspace tenant but are not added to backup, with coverage percent per client.

  _Use this to answer 'are all my users backed up?' for an insurer, QBR or new-hire check._

  ```bash
  dropsuite-cli coverage --agent
  ```
- **`billing`**  -  Compare each client's seat counters and shared mailboxes with what your PSA invoices, and flag over-allocation and suspended clients still holding seats.

  _Use this for month-end billing true-up instead of copying seat numbers client by client._

  ```bash
  dropsuite-cli billing --invoice psa-seats.csv --agent
  ```
- **`hygiene`**  -  Find deactivated or deleted mailboxes still holding storage and invitations nobody accepted, across every client.

  _Use this for the weekly offboarding pass and storage cleanup._

  ```bash
  dropsuite-cli hygiene --older-than 30d --agent
  ```
- **`locate`**  -  Find which client owns an email address, whether its mailbox and OneDrive are protected, and when they last backed up.

  _Use this for 'is this person backed up?' or restore tickets when you do not know which client they belong to._

  ```bash
  dropsuite-cli locate jane@example.com --agent
  ```

### Evidence that compounds locally
- **`evidence`**  -  Produce one client's dated proof of protection: coverage, last successful backups, item counts, retention policies, freshness history and recorded restore tests.

  _Use this for audit, cyber-insurance or QBR evidence; it never performs a restore._

  ```bash
  dropsuite-cli evidence --org 26561-14 --window 90d --markdown
  ```
- **`restore-test`**  -  Record the result of a restore test you performed in the portal and see which clients are due for one.

  _Use this to schedule and prove restore testing across clients; it records results, it does not restore, and due reads organizations from earlier health or sync runs._

  ```bash
  dropsuite-cli restore-test due --every 90d --pick --agent
  ```
- **`drift`**  -  See what changed between health runs: objects that left backup, backups that stopped advancing and item-count drops.

  _Use this to catch silent regressions that a single status check misses; it needs at least two earlier health runs._

  ```bash
  dropsuite-cli drift --since 7d --agent
  ```

## Known Limitations

- Not yet confirmed against a live partner tenant: built from the vendor's public API reference and tested against a local mock.
- Organization-scoped commands (accounts, onedrives, sharepoint-*, teams*, shared-drive*, google-drives, calendars, contacts, tasks, journals, portal) need `--org <id or owner email>` (or `DROPSUITE_ORG`) when `DROPSUITE_ACCESS_TOKEN` is the reseller token; with only one organization's token, the fleet commands fail on `organizations list`.
- Dropsuite answers HTTP 500 for organizations with no SharePoint or Teams data; `health` reports an unknown row after a short retry, so `--exit-code` can exit 3 when backups are fine; `--treat-500-as-empty` reads those as empty instead.
- Older partner hosts do not serve `/users/{id}/tenants`: `coverage` reports `unsupported-on-deployment`, `evidence` shows coverage as n/a, and `locate` falls back to the owner email domain.
- `drift` needs at least two earlier `health` runs; `evidence` freshness history needs earlier `health` runs; `restore-test due` reads organizations from earlier `health` or `sync` runs.
- Read-only, except that portal get-login-link, organizations get-support-sso-link, locate --sso-link and evidence --with-sso-link mint a portal sign-in URL (a login credential; a new organization link invalidates the previous one).
- `health`, `coverage` and `hygiene` skip deactivated or suspended customers unless `--include-inactive` is passed.
- Requests go only to the configured API host (`DROPSUITE_HOST` or `DROPSUITE_BASE_URL`); a redirect to any other host is refused. Output that repeats a held token, whole, encoded or split, prints `[text withheld: it repeated a secret value this program holds]` in its place.

## Command Reference

**accounts**  -  An organization's backed-up mailboxes (needs `--org`)

- `dropsuite-cli accounts list`  -  List an organization's backed-up mailboxes with their backup status and last backup.
- `dropsuite-cli accounts list-connection-failures`  -  List an organization's mailboxes whose backup cannot connect to the mail server.
- `dropsuite-cli accounts list-deactivated`  -  List an organization's mailboxes whose backup is deactivated.
- `dropsuite-cli accounts list-deleted`  -  List an organization's mailboxes that were deleted from backup.
- `dropsuite-cli accounts list-invites`  -  Show the pending backup invitation sent to a mailbox owner.

**calendars**  -  List calendars (needs `--org`)

- `dropsuite-cli calendars`  -  List an organization's calendar backups per mailbox.

**contacts**  -  List contacts (needs `--org`)

- `dropsuite-cli contacts`  -  List an organization's contact backups per mailbox.

**google-drives**  -  List google drives (needs `--org`)

- `dropsuite-cli google-drives`  -  List an organization's backed-up Google Drives.

**journals**  -  An organization's journal mailboxes (needs `--org`)

- `dropsuite-cli journals get`  -  Get the archiver journal mailbox of an organization (Archiver product only).
- `dropsuite-cli journals get-ndr`  -  Get the NDR (non-delivery report) journal mailbox of an organization.

**onedrives**  -  List onedrives (needs `--org`)

- `dropsuite-cli onedrives`  -  List an organization's backed-up OneDrive accounts with file counts and last backup.

**organizations**  -  List and get client organizations (subscription, tenants, delegated users, retention)

- `dropsuite-cli organizations get <id>`  -  Get one organization's subscription, including the seat counters used for billing.
- `dropsuite-cli organizations get-support-sso-link --email <user email>`  -  Generate a portal sign-in link for one user email, using the reseller's own token.
- `dropsuite-cli organizations list`  -  List every client organization (tenant user) under the reseller, with seats and storage counters.
- `dropsuite-cli organizations list-delegated-users <id>`  -  List an organization's delegated portal users and their roles.
- `dropsuite-cli organizations list-retention-policies <id>`  -  List an organization's retention policies.
- `dropsuite-cli organizations list-tenant-accounts <id> <tenant_id> --type m365`  -  List a tenant's mailboxes and whether each is backed up, available to add, or excluded.
- `dropsuite-cli organizations list-tenants <id>`  -  List an organization's Microsoft 365 and Google Workspace tenants.

**portal**  -  An organization's portal: sub-users, sign-in link, hosted Exchange status (needs `--org`)

- `dropsuite-cli portal get-hosted-exchange-status --exchange-request-id <id>`  -  Get the status of a hosted Exchange PowerShell connection request.
- `dropsuite-cli portal get-login-link`  -  Generate a single-use portal sign-in URL for an organization, for example to run a restore test.
- `dropsuite-cli portal list-sub-users`  -  List the sub-users who can sign in to an organization's portal, with their role.

**reference**  -  Reference lists: countries, time zones, languages

- `dropsuite-cli reference list-countries`  -  List the countries Dropsuite accepts.
- `dropsuite-cli reference list-locales`  -  List the portal languages as a map of language name to locale code (for example english to en).
- `dropsuite-cli reference list-timezones`  -  List the time zones Dropsuite accepts.

**server**  -  API server status

- `dropsuite-cli server`  -  Check that the Dropsuite API server is up. Wire format: GET /api/status; the response body is the JSON string OK.

**shared-drive-backups**  -  List shared drive backups (needs `--org`)

- `dropsuite-cli shared-drive-backups <id> <drive_id>`  -  List the backup runs of one Google shared drive.

**shared-drive-domains**  -  List shared drive domains (needs `--org`)

- `dropsuite-cli shared-drive-domains`  -  List an organization's Google Workspace shared drive domains.

**shared-drives**  -  List shared drives (needs `--org`)

- `dropsuite-cli shared-drives <id>`  -  List the shared drives in one Google Workspace domain.

**sharepoint-domains**  -  List sharepoint domains (needs `--org`)

- `dropsuite-cli sharepoint-domains`  -  List an organization's backed-up SharePoint domains.

**sharepoint-sites**  -  List sharepoint sites (needs `--org`)

- `dropsuite-cli sharepoint-sites <domain_id>`  -  List the backed-up SharePoint sites in one domain.

**tasks**  -  List tasks (needs `--org`)

- `dropsuite-cli tasks`  -  List an organization's task backups per mailbox.

**teams**  -  List teams (needs `--org`)

- `dropsuite-cli teams <domain_id>`  -  List the backed-up Teams and Microsoft 365 Groups in one domain.

**teams-domains**  -  List teams domains (needs `--org`)

- `dropsuite-cli teams-domains`  -  List an organization's backed-up Teams and Groups domains.


**billing**


- `dropsuite-cli billing`  -  Compare each client's seat counters and shared mailboxes with what your PSA invoices, and flag over-allocation and suspended clients still holding seats.


**coverage**


- `dropsuite-cli coverage`  -  List mailboxes that exist in each client's Microsoft 365 or Google Workspace tenant but are not added to backup, with coverage percent per client.


**drift**


- `dropsuite-cli drift`  -  See what changed between health runs: objects that left backup, backups that stopped advancing and item-count drops.


**evidence**


- `dropsuite-cli evidence`  -  Produce one client's dated proof of protection: coverage, last successful backups, item counts, retention policies, freshness history and recorded restore tests.


**health**


- `dropsuite-cli health`  -  See every failed, stuck or stale mailbox, OneDrive, SharePoint site, Team, shared drive and Google Drive across your active clients in one run.


**hygiene**


- `dropsuite-cli hygiene`  -  Find deactivated or deleted mailboxes still holding storage and invitations nobody accepted, across every client.


**locate**


- `dropsuite-cli locate`  -  Find which client owns an email address, whether its mailbox and OneDrive are protected, and when they last backed up.


**restore-test**


- `dropsuite-cli restore-test`  -  Record the result of a restore test you performed in the portal and see which clients are due for one.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
dropsuite-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Morning backup sweep

```bash
dropsuite-cli health --stale-after 24h --failing --agent
```

Graded rows for every failing, stuck or stale object across active clients; add --exit-code for monitoring (2 = crit, 3 = unknown, 1 = warn).

### Seat counters for every client

```bash
dropsuite-cli organizations list --agent --select id,email,seats_used,seats_available,customer_deactivated
```

Narrows the organization records to their seat counters; billing adds active, required and deactivated seats and shared mailboxes.

### Month-end billing true-up

```bash
dropsuite-cli billing --invoice psa-seats.csv --flagged-only --agent
```

The CSV needs a key column (external_id, org_id, id or email) and a quantity column; pick the billed counter with --billable-field (default seats_used, which Dropsuite does not document as the billed one).

### Insurance evidence for one client

```bash
dropsuite-cli evidence --org 26561-14 --window 90d --markdown
```

Dated proof of coverage, last backups, retention and recorded restore tests; freshness history needs earlier health runs.

### Log a restore test

```bash
dropsuite-cli restore-test record --org 26561-14 --object jane@example.com --kind mailbox --result pass --note "restored 3 messages"
```

Records a restore you performed in the portal; restore-test due --every 90d --pick lists clients that need one.

## Auth Setup

Every call sends two headers from Settings > API Settings: X-Reseller-Token (DROPSUITE_RESELLER_TOKEN) and X-Access-Token (DROPSUITE_ACCESS_TOKEN, your reseller Authentication Token). GET /users and GET /users/{id} take that reseller token. Everything else (backup data such as accounts and onedrives, and an organization's tenants, delegated users and retention policies) takes one organization's own token: pass --org <organization id or owner email> and the CLI resolves it from the organizations list, or set DROPSUITE_ORG to route an MCP server or shell to one organization. The fleet commands (health, coverage, billing, hygiene, locate, evidence) resolve every organization's token themselves and need the reseller token. The API URL is per partner: set DROPSUITE_HOST to its host (default dropsuite.us), or DROPSUITE_BASE_URL to the whole URL. The CLI only reads, except that portal get-login-link, organizations get-support-sso-link, locate --sso-link and evidence --with-sso-link mint a portal sign-in URL (a login credential; a new organization link invalidates the previous one). Every token the CLI holds (both reseller tokens, and each organization token it reads from GET /users) is withheld from all output, files and the local store in any encoding; there is no response cache; and requests, redirects included, go only to the configured API host. Over MCP, the sign-in link calls return a preview until confirm is true.

Run `dropsuite-cli doctor` to verify setup.

`--ca-cert`, `DROPSUITE_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `DROPSUITE_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

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
  dropsuite-cli accounts list --org 26561-14 --agent --select added_on,current_backup_status,deactivated_since
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit confirmation**  -  `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear

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

- Use `--home <dir>` for one invocation, or set `DROPSUITE_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `DROPSUITE_CONFIG_DIR`, `DROPSUITE_DATA_DIR`, `DROPSUITE_STATE_DIR`, `DROPSUITE_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `DROPSUITE_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, and auth sidecars. `state` contains persisted queries and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `dropsuite-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "dropsuite": {
        "command": "dropsuite-mcp",
        "env": {
          "DROPSUITE_HOME": "/srv/dropsuite"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `DROPSUITE_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `DROPSUITE_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; The response cache is off: no API response is written under the cache dir. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `DROPSUITE_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `DROPSUITE_DATA_SOURCE` sets the `--data-source` default.

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
dropsuite-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "dropsuite-cli learnings confirm 12"] }
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
       materially more, record the divergence via `dropsuite-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `dropsuite-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "groups" → Teams and Groups + Microsoft 365 Groups). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `dropsuite-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
dropsuite-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "mailbox") satisfies future queries under another alias (e.g., "exchange", "email accounts") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
dropsuite-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
dropsuite-cli teach-playbook \
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
dropsuite-cli playbook amend \
  --query "$QUERY" \
  --add-note "$NOTE"
# (append shell `&` to background it)
```

What counts as worth amending: a behavior you OBSERVED this session that future-you would benefit from knowing. Examples worth amending:

- A workaround for a CLI surface that silently drops or misorders a flag.
- An undocumented endpoint shape (response wrapped in `{meta, results}`, payload nested two levels deeper than the docs claim).
- Observed schema drift (a field renamed, an index that shifted between API versions, a category label that the API now returns lower-cased).

What does NOT belong in notes:

- The year-specific or entity-specific answer to the user's question. That's the response, not a learning.
- Per-organization / per-mailbox / per-row data the playbook already retrieves at runtime.
- Statements that paraphrase what the existing notes already say.

The amend command appends to the family's existing notes with a timestamped marker (`[amend YYYY-MM-DDTHH:MMZ]: <text>`). Multiple amends accumulate; the audit trail is visible. If no playbook exists yet for the family, amend creates a notes-only one (so cold-start corrections still land).

#### PII discipline for amend notes

`playbook amend` notes are designed to potentially flow upstream as shared knowledge in future versions of the Printing Press. Keep them clean of user-identifying content so the upstream-contribution path stays open without retroactive scrubbing:

- **Do NOT embed** paths to user filesystems, personal API keys or tokens, user email addresses, user GitHub handles, or specific query histories tied to a single user.
- **Acceptable**: endpoint shapes, undocumented field names, API gotchas, observed schema drift, workarounds for CLI surfaces, generalizable pagination or retry tactics.

If a correction is only meaningful with user-specific context, it belongs in a personal note, not in the playbook amend.

### Measuring the loop

`dropsuite-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `DROPSUITE_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
dropsuite-cli feedback "the --since flag is inclusive but docs say exclusive"
dropsuite-cli feedback --stdin < notes.txt
dropsuite-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `DROPSUITE_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `DROPSUITE_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
dropsuite-cli profile save briefing --json
dropsuite-cli --profile briefing health --failing
dropsuite-cli profile list --json
dropsuite-cli profile show briefing
dropsuite-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Generic error; with `health --exit-code`, at least one warn row |
| 2 | Usage error (wrong arguments); with `health --exit-code`, at least one crit row |
| 3 | Resource not found; with `health --exit-code`, at least one unknown row |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `dropsuite-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add dropsuite-mcp \
     -e DROPSUITE_RESELLER_TOKEN=<reseller-token> \
     -e DROPSUITE_ACCESS_TOKEN=<authentication-token> \
     -e DROPSUITE_HOST=dropsuite.us \
     -- dropsuite-mcp
   ```
   Add `-e DROPSUITE_ORG=<organization id or owner email>` to point the per-organization tools at one client.
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which dropsuite-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   dropsuite-cli health --failing --agent
   ```
4. If ambiguous, drill into subcommand help, for example `dropsuite-cli coverage --help`.
