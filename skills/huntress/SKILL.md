---
name: huntress
description: "Every Huntress endpoint, plus fleet-wide incident, coverage, and billing rollups the API can't. Trigger phrases: `show me all critical huntress incidents`, `huntress coverage gaps across my orgs`, `huntress blast radius for this IP`, `reconcile huntress billing`, `huntress agent health report`, `use huntress`, `run huntress`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Huntress"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - huntress-cli
    install:
      - kind: script
        bins: [huntress-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/huntress/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/huntress/install.ps1
---

# Huntress - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `huntress-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/huntress/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/huntress/install.ps1 | iex
   ```
3. Verify: `huntress-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `huntress-cli` and `huntress-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

huntress-cli absorbs the full Huntress API  -  organizations, agents, incident reports, remediations, signals, escalations, identities, external recon, reports, invoices, reseller subscriptions, and SIEM ES|QL  -  with agent-native output (--json, --select, typed exit codes). Then it transcends the read-mostly, per-org API: fleet-incidents gives one age-sorted queue across every client org, coverage-gaps rolls up posture exposure, blast-radius correlates an indicator across the whole fleet, drift keeps the agent snapshot history the point-in-time API lacks, and mttr and handoff compute resolution times and shift changes from synced incident timestamps.

## When to Use This CLI

Reach for huntress-cli when an agent or analyst needs to act across an entire Huntress account from the terminal: triaging incidents across many client organizations, auditing agent coverage and posture, correlating an indicator fleet-wide during an incident, reconciling billing against deployed agents, or pulling QBR/shift-handoff rollups. It is the right tool whenever the question spans more than one org or needs history the live API doesn't keep.

## Anti-triggers

Do not use this CLI for:
- Single-record point-in-time lookups (one org, one agent, one incident by id)  -  use the generated resource get commands, not the fleet rollups
- Non-Huntress security tooling questions (other EDR/RMM/SIEM vendors)  -  this CLI only speaks the Huntress API
- Cross-Huntress-account queries with standard credentials  -  the API credential scopes one account; only reseller-scoped keys see multiple accounts
- Real-time streaming/alerting  -  the local store is sync-cadence fresh, not a live event feed; use Huntress webhooks for push alerts

## Unique Capabilities

### Fleet rollups across every org
- **`fleet-incidents`**  -  One unified, age-sorted incident queue across every client organization, with org names joined in  -  the morning-sweep view the dashboard can't give.

  _Reach for this when an agent needs the single cross-tenant 'what's on fire everywhere' queue instead of paging org-by-org._

  ```bash
  huntress-cli fleet-incidents --severity critical --status sent --sort age --json
  ```
- **`coverage-gaps`**  -  Per-org rollup of agent health: for each org with a gap, how many agents have a stale callback, an unreadable callback time, or Defender or firewall not reported healthy, worst-first.

  _Use before a weekly posture review or onboarding audit to find protection gaps before an incident lands on an unmonitored host._

  ```bash
  huntress-cli coverage-gaps --stale-days 7 --json
  ```
- **`canary-watch`**  -  Surfaces only ransomware-canary and foothold incidents in a time window  -  the highest-signal early-ransomware indicators, fleet-wide.

  _Reach for this as a high-signal early-warning sweep when you want only the indicators that precede ransomware._

  ```bash
  huntress-cli canary-watch --window 24h --json
  ```
- **`stale-agents`**  -  Lists agents whose last callback exceeds a threshold  -  decommissioned-but-billed machines or broken installs.

  _Use to clean up the fleet and to explain a billing-reconcile delta with the agents that stopped reporting._

  ```bash
  huntress-cli stale-agents --days 14 --platform windows --json
  ```
- **`fleet-summary`**  -  One-screen fleet top-line: total orgs and agents, open incidents, open criticals, the oldest open critical's age, stale agents, and orgs with zero agents.

  _Run this first each shift  -  it tells an agent which detail command (fleet-incidents, coverage-gaps, stale-agents) to reach for next._

  ```bash
  huntress-cli fleet-summary --agent
  ```
- **`signal-digest`**  -  Groups SOC signals by type and organization over a time window, with reported-versus-closed counts: what the Huntress SOC saw across the fleet.

  _Reach for this before a weekly client touchpoint or QBR to say which detection types fired where, without paging signals org by org._

  ```bash
  huntress-cli signal-digest --since 7d --json
  ```

### Cross-entity correlation
- **`blast-radius`**  -  Given an indicator (IP, MAC, domain or hostname, file hash, or text fragment), finds the matching incident reports, agents, and external ports across every org - instant correlation during incident response.

  _Reach for this mid-incident to answer 'where else does this indicator appear across my whole fleet' in one call._

  ```bash
  huntress-cli blast-radius --indicator 203.0.113.7 --json
  ```
- **`triage-age`**  -  SLA aging report: open incidents bucketed by hours open, each bucket broken out by severity; the oldest bucket is flagged as an SLA breach when any incident has aged into it.

  _Use to enforce response SLAs across tenants: the oldest bucket counts, by severity, the open incidents past your SLA._

  ```bash
  huntress-cli triage-age --buckets 4,24,72 --json
  ```
- **`org-scorecard`**  -  Per-client QBR rollup for one org: agent count, open, closed and total incidents, and mean time-to-resolve.

  _Use to assemble a client quarterly-business-review summary in one command instead of stitching several endpoints._

  ```bash
  huntress-cli org-scorecard --org 4821 --json
  ```
- **`incident-detail`**  -  See one incident fully enriched  -  its remediations, the affected agent, and the org name  -  in a single lookup.

  _Reach for this when triaging a specific incident: one call replaces the incident/remediation/agent/org fan-out._

  ```bash
  huntress-cli incident-detail 123456 --agent
  ```
- **`escalation-queue`**  -  Unresolved escalations across the fleet, critical first and oldest first, with hours open and the organizations each one names: the environmental-issue backlog the incident queue does not show.

  _Use in the morning sweep next to fleet-incidents so Defender-disabled and unwanted-access escalations do not sit unresolved._

  ```bash
  huntress-cli escalation-queue --severity critical --json
  ```

### Partner ops and billing
- **`billing-reconcile`**  -  Compares the seats on each account's latest invoice, summed, with the agents in the local mirror and reports the delta.

  _Run at monthly close to catch decommissioned-but-billed seats and under-billed new deployments._

  ```bash
  huntress-cli billing-reconcile --json
  ```
- **`reseller-rollup`**  -  Per-account roll-up for resellers: subscription count and products, minimum usage by product, deployed agent count, and deployed agents against the active EDR minimum.

  _Live-only: calls the API directly and requires reseller-scoped credentials (standard account keys get 401; no sync needed). Use at month close for multi-account resellers; for invoiced seats against deployed agents inside one account use billing-reconcile instead._

  ```bash
  huntress-cli reseller-rollup --json
  ```

### Local history that compounds
- **`drift`**  -  Diffs the agent list against the snapshot its previous run saved: agents added, agents removed, and agents whose last callback has not moved since then.

  _Run after each sync to see which agents were added, removed, or went quiet since the previous drift run._

  ```bash
  huntress-cli drift --entity agents --json
  ```
- **`mttr`**  -  Computes mean time-to-resolve for incidents from sent-to-resolved timestamps, grouped by org or severity.

  _Use for QBR and SOC performance reporting where mean response time is the headline number._

  ```bash
  huntress-cli mttr --group-by org --since 30d --json
  ```
- **`handoff`**  -  Shift-change report of what happened in a look-back window (new incidents, resolved incidents, new escalations), ready to paste into a handoff note.

  _Use at shift change so the next analyst sees exactly what moved during the prior shift._

  ```bash
  huntress-cli handoff --since 8h --json
  ```

## Command Reference

Commands below that change the tenant (create, update, delete, resolution, remediation approval or rejection, subscription changes) refuse to run without `--yes` or `--dry-run`. Preview first, for example `huntress-cli organizations update-parameters 12345 --name Acme --dry-run`, then run the same command with `--yes`.

**account**  -  Operations about Accounts

- `huntress-cli account`  -  Shows details of the top-level Huntress Account associated with your API credentials.

**accounts**  -  Operations about Accounts

- `huntress-cli accounts creation-parameters`  -  Create a new account under the reseller associated with the supplied API credential.
- `huntress-cli accounts delete-v1-id`  -  Marks the account as disabled and will be deleted after 10 days from initial request.
- `huntress-cli accounts get-v1`  -  Shows all accounts associated with your API credentials.
- `huntress-cli accounts get-v1-id`  -  Shows the details of a specific account which your API credentials grant access to.
- `huntress-cli accounts update-parameters`  -  Updates the details of a specific account.

**actor**  -  Operations about Actors

- `huntress-cli actor`  -  Shows details of the entities associated with the supplied API credentials.

**agents**  -  Operations about Agents

- `huntress-cli agents get-v1`  -  Shows Agents associated with your account.
- `huntress-cli agents get-v1-id`  -  Shows details on a single Agent associated with your account.

**escalations**  -  Operations about Escalations

- `huntress-cli escalations get-v1`  -  Shows Escalations associated with your account.
- `huntress-cli escalations get-v1-id`  -  Shows details on a single Escalation associated with your account.

**external-ports**  -  Manage external ports

- `huntress-cli external-ports get-v1`  -  Shows external port records from External Recon scans associated with your account.
- `huntress-cli external-ports get-v1-id`  -  Shows details on a single external port record associated with your account.

**identities**  -  Operations about Identities

- `huntress-cli identities get-v1`  -  Shows Identities associated with your account.
- `huntress-cli identities get-v1-id`  -  Shows details on a single Identity associated with your account.

**incident-reports**  -  Operations about Incident Reports

- `huntress-cli incident-reports get-v1`  -  Shows Incident Reports associated with your account.
- `huntress-cli incident-reports get-v1-id`  -  Shows details on a single Incident Report associated with your account.

**invoices**  -  Operations about Invoices

- `huntress-cli invoices get-v1`  -  Shows Invoices associated with your account.
- `huntress-cli invoices get-v1-id`  -  Shows details on a single Invoice associated with your account.

**known-vpns**  -  Operations about Known VPNs

- `huntress-cli known-vpns`  -  Returns the list of VPN and proxy operators recognized by Huntress.

**memberships**  -  Manage memberships

- `huntress-cli memberships creation-parameters`  -  This endpoint allows you to invite a user to join your organization or account.
- `huntress-cli memberships delete-v1-id`  -  Deletes a single Membership associated with your account or organization.
- `huntress-cli memberships get-v1`  -  Shows a list of memberships.
- `huntress-cli memberships get-v1-id`  -  Shows details on a single Membership associated with your account or organization.
- `huntress-cli memberships update-parameters`  -  Update a User's membership

**organizations**  -  Operations about Organizations

- `huntress-cli organizations creation-parameters`  -  Create an Organization
- `huntress-cli organizations delete-v1-id`  -  Deletes the specified Organization.
- `huntress-cli organizations get-v1`  -  Shows details of Organizations belonging to the account associated with your API credentials.
- `huntress-cli organizations get-v1-id`  -  Shows details on a single Organization associated with your account.
- `huntress-cli organizations update-parameters`  -  Update an Organization

**reports**  -  Manage reports

- `huntress-cli reports get-v1`  -  Shows Summary Reports associated with your account.
- `huntress-cli reports get-v1-id`  -  Shows details on a single Summary Report associated with your account.

**reseller**  -  Operations for Reseller-level API credentials. These are mostly the same endpoints available in the rest of the API. However, the account ID is included in the URL, so that you can specify which account's resources you want to access.

- `huntress-cli reseller get-v1-invoices`  -  Shows Invoices associated with the current reseller.
- `huntress-cli reseller get-v1-invoices-id`  -  Shows a specific Reseller Invoice associated with the current reseller.
- `huntress-cli reseller get-v1-invoices-id-account-usage-line-items`  -  Shows a list of Account Usage Line Items.
- `huntress-cli reseller get-v1-invoices-id-organization-usage-line-items`  -  Shows a list of Organization Usage Line Items.
- `huntress-cli reseller get-v1-subscriptions`  -  Shows subscriptions associated with the current reseller's managed accounts.
- `huntress-cli reseller get-v1-subscriptions-id`  -  Shows details on a single subscription associated with the current reseller's managed accounts.
- `huntress-cli reseller subscription-creation-parameters`  -  Creates a subscription for a product on a reseller-managed account.
- `huntress-cli reseller subscription-update-parameters`  -  Updates a subscription associated with the current reseller's managed accounts.
- `huntress-cli reseller subscription-upgrade-parameters`  -  Upgrades an active subscription by creating a new subscription with a higher minimum and/or price tier

**siem**  -  Query your SIEM logs programmatically using <a href="https://support.huntress.io/hc/en-us/articles/30113222043155-Searching-Logs-ESQL">ES|QL (Elasticsearch Query Language)</a>.

- `huntress-cli siem`  -  Execute an ESQL query against your SIEM logs and receive paginated JSON results.

**signals**  -  Operations about Signals

- `huntress-cli signals get-v1`  -  Shows details of Signals belonging to the account associated with your API credentials.
- `huntress-cli signals get-v1-id`  -  Shows details of a single Signal belonging to the account associated with your API credentials.

**unwanted-access-rules**  -  Operations about Unwanted Access Rules

- `huntress-cli unwanted-access-rules creation-parameters`  -  Creates a new Unwanted Access Rule associated with your account, an organization, or a specific identity. **Rule logic.
- `huntress-cli unwanted-access-rules delete-v1-id`  -  Deletes a single Unwanted Access Rule associated with your account.
- `huntress-cli unwanted-access-rules get-v1`  -  Shows Unwanted Access Rules associated with your account.
- `huntress-cli unwanted-access-rules get-v1-id`  -  Shows details on a single Unwanted Access Rule associated with your account.
- `huntress-cli unwanted-access-rules update-parameters`  -  Updates the schedule and notes on an existing Unwanted Access Rule.


**billing-reconcile**


- `huntress-cli billing-reconcile`  -  Compares the seats on each account's latest invoice, summed, with the agents in the local mirror and reports the delta.


**blast-radius**


- `huntress-cli blast-radius`  -  Given an indicator (IP, MAC, domain or hostname, file hash, or text fragment), finds the matching incident reports, agents, and external ports across every org - instant correlation during incident response.


**canary-watch**


- `huntress-cli canary-watch`  -  Surfaces only ransomware-canary and foothold incidents in a time window  -  the highest-signal early-ransomware indicators, fleet-wide.


**coverage-gaps**


- `huntress-cli coverage-gaps`  -  Per-org rollup of agent health: for each org with a gap, how many agents have a stale callback, an unreadable callback time, or Defender or firewall not reported healthy, worst-first.


**drift**


- `huntress-cli drift`  -  Diffs the agent list against the snapshot its previous run saved: agents added, agents removed, and agents whose last callback has not moved since then.


**escalation-queue**


- `huntress-cli escalation-queue`  -  Unresolved escalations across the fleet, critical first and oldest first, with hours open and the organizations each one names: the environmental-issue backlog the incident queue does not show.


**fleet-incidents**


- `huntress-cli fleet-incidents`  -  One unified, age-sorted incident queue across every client organization, with org names joined in  -  the morning-sweep view the dashboard can't give.


**fleet-summary**


- `huntress-cli fleet-summary`  -  One-screen fleet top-line: total orgs and agents, open incidents, open criticals, the oldest open critical's age, stale agents, and orgs with zero agents.


**handoff**


- `huntress-cli handoff`  -  Shift-change report of what happened in a look-back window (new incidents, resolved incidents, new escalations), ready to paste into a handoff note.


**incident-detail**


- `huntress-cli incident-detail`  -  See one incident fully enriched  -  its remediations, the affected agent, and the org name  -  in a single lookup.


**mttr**


- `huntress-cli mttr`  -  Computes mean time-to-resolve for incidents from sent-to-resolved timestamps, grouped by org or severity.


**org-scorecard**


- `huntress-cli org-scorecard`  -  Per-client QBR rollup for one org: agent count, open, closed and total incidents, and mean time-to-resolve.


**reseller-rollup**


- `huntress-cli reseller-rollup`  -  Per-account roll-up for resellers: subscription count and products, minimum usage by product, deployed agent count, and deployed agents against the active EDR minimum.


**signal-digest**


- `huntress-cli signal-digest`  -  Groups SOC signals by type and organization over a time window, with reported-versus-closed counts: what the Huntress SOC saw across the fleet.


**stale-agents**


- `huntress-cli stale-agents`  -  Lists agents whose last callback exceeds a threshold  -  decommissioned-but-billed machines or broken installs.


**triage-age**


- `huntress-cli triage-age`  -  SLA aging report: open incidents bucketed by hours open, each bucket broken out by severity; the oldest bucket is flagged as an SLA breach when any incident has aged into it.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
huntress-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Cross-tenant critical queue

```bash
huntress-cli fleet-incidents --severity critical --status sent --sort age --json
```

One age-sorted queue of every open critical across all client orgs.

### Posture exposure sweep

```bash
huntress-cli coverage-gaps --stale-days 7 --json
```

Per-org counts of agents with a stale callback or Defender or firewall not reported healthy, worst-first.

### Indicator correlation during IR

```bash
huntress-cli blast-radius --indicator 203.0.113.7 --json
```

The incident reports, agents, and external ports, across every org, that match the IP, hash, or other indicator, each with how it matched.

### Trim verbose fleet-incident payloads

```bash
huntress-cli fleet-incidents --status sent --agent --select organization_name,severity,hours_open,indicator_types
```

fleet-incidents joins org names and returns a wide row per incident; --agent with --select returns only the fields you need so agents don't burn context.

### Billing true-up at month close

```bash
huntress-cli billing-reconcile --json
```

Seats on each account's latest invoice, summed, versus the agents in the local mirror, with the delta (`invoiced_seats`, `deployed_agents`, `delta`).

## Auth Setup

Huntress uses HTTP Basic auth: set HUNTRESS_API_KEY and HUNTRESS_API_SECRET (minted in Account Settings, or reseller-level for multi-account partners). The CLI composes the Base64 Authorization header for you. Most data is read-only; resolve/approve/reject and CRUD commands need a credential with write scope.

Run `huntress-cli doctor` to verify setup.

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
  huntress-cli account --agent --select billing_address,id,name
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit confirmation**  -  every command that changes the tenant (create, update, delete, incident resolution, remediation approval or rejection, subscription changes, `import`) refuses to run and sends nothing without `--yes` (send) or `--dry-run` (preview); the refusal exits 2 with `"confirmation_required": true` in the JSON envelope. `--agent` does not imply `--yes`; pass `--yes` only after the target, arguments, and side effects are clear
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

- Use `--home <dir>` for one invocation, or set `HUNTRESS_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `HUNTRESS_CONFIG_DIR`, `HUNTRESS_DATA_DIR`, `HUNTRESS_STATE_DIR`, `HUNTRESS_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `HUNTRESS_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `huntress-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "huntress": {
        "command": "huntress-mcp",
        "env": {
          "HUNTRESS_HOME": "/srv/huntress"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `HUNTRESS_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `HUNTRESS_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are cached for five minutes under the cache dir. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `HUNTRESS_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `HUNTRESS_DATA_SOURCE` sets the `--data-source` default.

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
huntress-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "huntress-cli learnings confirm 12"] }
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
       materially more, record the divergence via `huntress-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `huntress-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `huntress-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
huntress-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
huntress-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
huntress-cli teach-playbook \
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
huntress-cli playbook amend \
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

`huntress-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `HUNTRESS_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
huntress-cli feedback "the --since flag is inclusive but docs say exclusive"
huntress-cli feedback --stdin < notes.txt
huntress-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `HUNTRESS_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `HUNTRESS_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
huntress-cli profile save briefing --json
huntress-cli --profile briefing account
huntress-cli profile list --json
huntress-cli profile show briefing
huntress-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `huntress-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add huntress-mcp -- huntress-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `huntress-mcp --transport http` requires `HUNTRESS_MCP_HTTP_TOKEN` (callers send `Authorization: Bearer <token>`), serves `http://127.0.0.1:7777/mcp` by default, needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`, and refuses a request body over 4 MiB or one that takes longer than 30 seconds to arrive. `HUNTRESS_MCP_READ_ONLY=1` lists only read-only tools: it removes `huntress_execute`, `sync`, `import`, `export`, `workflow_archive` and the learn-store writers (`teach`, `teach_lookup`, `teach_pattern`, `teach_playbook`, `playbook_amend`, `learnings_confirm`, `learnings_reject`, `learnings_forget`), 13 of the 42 tools, and `huntress_read` still refuses every state-changing endpoint.

## Direct Use

1. Check if installed: `which huntress-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   huntress-cli <command> [subcommand] [args] --agent
   ```
   A command that changes the tenant refuses without `--yes` or `--dry-run`: run it with `--dry-run` first, show the user the previewed request, and add `--yes` only after they approve.
4. If ambiguous, drill into subcommand help: `huntress-cli <command> --help`.
