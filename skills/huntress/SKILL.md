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
      - kind: go
        bins: [huntress-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/huntress/cmd/huntress-cli
---

# Huntress  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `huntress-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install huntress --cli-only
   ```
2. Verify: `huntress-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/huntress/cmd/huntress-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

huntress-cli absorbs the full Huntress API  -  organizations, agents, incident reports, remediations, signals, escalations, identities, external recon, reports, invoices, reseller subscriptions, and SIEM ES|QL  -  with agent-native output (--json, --select, typed exit codes). Then it transcends the read-mostly, per-org API: fleet-incidents gives one age-sorted queue across every client org, coverage-gaps rolls up posture exposure, blast-radius correlates an indicator across the whole fleet, and drift/mttr/handoff turn repeated syncs into history the live API throws away.

## When to Use This CLI

Reach for huntress-cli when an agent or analyst needs to act across an entire Huntress account from the terminal: triaging incidents across many client organizations, auditing agent coverage and posture, correlating an indicator fleet-wide during an incident, reconciling billing against deployed agents, or pulling QBR/shift-handoff rollups. It is the right tool whenever the question spans more than one org or needs history the live API doesn't keep.

## Anti-triggers

Do not use this CLI for:
- Single-record point-in-time lookups (one org, one agent, one incident by id)  -  use the generated resource get commands, not the fleet rollups
- Non-Huntress security tooling questions (other EDR/RMM/SIEM vendors)  -  this CLI only speaks the Huntress API
- Cross-Huntress-account queries with standard credentials  -  the API credential scopes one account; only reseller-scoped keys see multiple accounts
- Real-time streaming/alerting  -  the local store is sync-cadence fresh, not a live event feed; use Huntress webhooks for push alerts

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Fleet rollups across every org
- **`fleet-incidents`**  -  One unified, age-sorted incident queue across every client organization, with org names joined in  -  the morning-sweep view the dashboard can't give.

  _Reach for this when an agent needs the single cross-tenant 'what's on fire everywhere' queue instead of paging org-by-org._

  ```bash
  huntress-cli fleet-incidents --severity critical --status sent --sort age --json
  ```
- **`coverage-gaps`**  -  Flags orgs and agents with stale callbacks, unhealthy Defender/firewall, or outdated EDR versions  -  a fleet posture exposure report.

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

  _Use to clean up the fleet and feed billing-reconcile with the list of agents that stopped reporting._

  ```bash
  huntress-cli stale-agents --days 14 --platform windows --json
  ```
- **`fleet-summary`**  -  One-screen fleet top-line: total orgs, agents, open criticals, oldest unactioned critical, orgs below coverage threshold, stale agents.

  _Run this first each shift  -  it tells an agent which detail command (fleet-incidents, coverage-gaps, stale-agents) to reach for next._

  ```bash
  huntress-cli fleet-summary --agent
  ```

### Cross-entity correlation
- **`blast-radius`**  -  Given an indicator (external IP, file hash, or foothold signature), finds every agent, org, and incident that matches it  -  instant correlation during incident response.

  _Reach for this mid-incident to answer 'where else does this indicator appear across my whole fleet' in one call._

  ```bash
  huntress-cli blast-radius --indicator 203.0.113.7 --json
  ```
- **`triage-age`**  -  SLA aging report: open incidents bucketed by hours-open, broken out by org and severity, with breaches flagged.

  _Use to enforce response SLAs across tenants and surface the oldest unactioned criticals first._

  ```bash
  huntress-cli triage-age --buckets 4,24,72 --json
  ```
- **`org-scorecard`**  -  Per-client QBR scorecard: agent count, coverage percent, open and closed incidents, MTTR, and a posture grade  -  one client's security story.

  _Use to assemble a client quarterly-business-review summary in one command instead of stitching several endpoints._

  ```bash
  huntress-cli org-scorecard --org 4821 --json
  ```
- **`incident-detail`**  -  See one incident fully enriched  -  its remediations, the affected agent, and the org name  -  in a single lookup.

  _Reach for this when triaging a specific incident: one call replaces the incident/remediation/agent/org fan-out._

  ```bash
  huntress-cli incident-detail 123456 --agent
  ```

### Partner ops and billing
- **`billing-reconcile`**  -  Compares invoiced and subscribed seat counts against actually deployed agent counts per org and surfaces the delta.

  _Run at monthly close to catch decommissioned-but-billed seats and under-billed new deployments._

  ```bash
  huntress-cli billing-reconcile --json
  ```
- **`reseller-rollup`**  -  Per-account roll-up for resellers: invoice total, subscribed seats, and deployed agent count side by side.

  _Live-only: calls the API directly and requires reseller-scoped credentials (standard account keys get 401; no sync needed). Use at month close for multi-account resellers; for per-org drift inside one account use billing-reconcile instead._

  ```bash
  huntress-cli reseller-rollup --json
  ```

### Local history that compounds
- **`drift`**  -  Diffs the current sync against the prior snapshot: new and removed agents, status flips, new criticals, and version changes.

  _Use after each sync to see what changed across the fleet without re-reading every record._

  ```bash
  huntress-cli drift --entity agents --json
  ```
- **`mttr`**  -  Computes mean time-to-resolve for incidents from sent-to-resolved timestamps, grouped by org or severity.

  _Use for QBR and SOC performance reporting where mean response time is the headline number._

  ```bash
  huntress-cli mttr --group-by org --since 30d --json
  ```
- **`handoff`**  -  Shift-change report of what changed (new criticals, resolutions, escalations) since a timestamp, ready to paste into a handoff note.

  _Use at shift change so the next analyst sees exactly what moved during the prior shift._

  ```bash
  huntress-cli handoff --since 8h --json
  ```

## Command Reference

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


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
huntress-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

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

Orgs and agents with stale callbacks or unhealthy Defender/firewall, rolled up per org.

### Indicator correlation during IR

```bash
huntress-cli blast-radius --indicator 203.0.113.7 --json
```

Every agent, org, and incident touching an external IP, hash, or foothold.

### Trim verbose fleet-incident payloads

```bash
huntress-cli fleet-incidents --status sent --agent --select organization_name,severity,hours_open,indicator_types
```

fleet-incidents joins org names and returns a wide row per incident; --agent with --select returns only the fields you need so agents don't burn context.

### Billing true-up at month close

```bash
huntress-cli billing-reconcile --json
```

Invoiced/subscribed seats versus actually deployed agents per org, with the delta.

## Auth Setup

Huntress uses HTTP Basic auth: set HUNTRESS_API_KEY and HUNTRESS_API_SECRET (minted in Account Settings, or reseller-level for multi-account partners). The CLI composes the Base64 Authorization header for you. Most data is read-only; resolve/approve/reject and CRUD commands need a credential with write scope.

Run `huntress-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  huntress-cli account --agent --select id,name,status
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success, and `--ignore-missing` only when a missing delete target should count as success

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set  -  piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
huntress-cli feedback "the --since flag is inclusive but docs say exclusive"
huntress-cli feedback --stdin < notes.txt
huntress-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/huntress-cli/feedback.jsonl`. They are never POSTed unless `HUNTRESS_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `HUNTRESS_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename) |
| `webhook:<url>` | POST the output body to the URL (`application/json` or `application/x-ndjson` when `--compact`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled agent calls the same command every run with the same configuration - HeyGen's "Beacon" pattern.

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
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `huntress-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/huntress/cmd/huntress-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add huntress-mcp -- huntress-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which huntress-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   huntress-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `huntress-cli <command> --help`.
