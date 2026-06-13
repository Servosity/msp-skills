---
name: skykick
description: "Fleet-wide M365 backup assurance for SkyKick Cloud Backup - posture, stale snapshots, and coverage gaps no portal or wrapper can show. Trigger phrases: `check skykick backups`, `which customers aren't backed up`, `stale skykick snapshots`, `skykick fleet health`, `skykick backup alerts`, `audit m365 backup retention`, `use skykick`, `run skykick-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "SkyKick"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - skykick-cli
    install:
      - kind: go
        bins: [skykick-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/skykick/cmd/skykick-cli
---

# SkyKick  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `skykick-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install skykick --cli-only
   ```
2. Verify: `skykick-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/skykick/cmd/skykick-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Every evidenced SkyKick (ConnectWise Cloud Services) Backup API operation as typed commands, plus a local SQLite fleet store that answers the questions the per-tenant API can't: which customers aren't fully protected (fleet-health), which mailboxes silently stopped snapshotting (stale-snapshots), and what changed since last review (drift). Built for the current apis.cloudservices.connectwise.com host - the only CLI that works post-migration.

## When to Use This CLI

Reach for this CLI when a task touches SkyKick / ConnectWise Cloud Services M365 backup at fleet scale: auditing which customer tenants are protected, finding stale mailbox snapshots, reconciling discovered-vs-enabled coverage after onboarding, sweeping and bulk-completing alerts, or producing QBR-ready retention and autodiscover compliance tables. Single-tenant reads (settings, mailboxes, sites, SKU) work too, with --json/--select for agent pipelines.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI to create or manage SkyKick MIGRATION orders - the Migrate/Manage API families are login-gated and not exposed here
- Do not use it to enable/disable backup for a mailbox or site - the public API exposes no enablement writes; use the SkyKick portal
- Do not use it for Axcient, Datto, or other ConnectWise BDR products - this covers only the SkyKick Cloud Backup API
- Do not use it to restore backed-up data - restore operations are portal-only

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Fleet posture
- **`fleet-sync`**  -  One command pulls every subscription plus per-tenant settings, retention, autodiscover, snapshot stats, mailboxes, sites, and alerts into the local SQLite fleet store.

  _Run this first; every fleet-posture and backup-integrity command reads the store it builds._

  ```bash
  skykick-cli fleet-sync --agent
  ```
- **`fleet-health`**  -  One cross-tenant protection posture table - every subscription's Exchange/SharePoint enablement, retention, autodiscover, and last-backup age with gap flags.

  _The single command an MSP owner runs to see fleet-wide backup posture without per-tenant portal crawling._

  ```bash
  skykick-cli fleet-health --flag-gaps --agent
  ```
- **`retention-audit`**  -  Grades each tenant's retention period against a compliance floor you set.

  _Turns scattered retention numbers into a pass/under-floor compliance attestation for QBRs._

  ```bash
  skykick-cli retention-audit --floor-days 365 --agent
  ```
- **`autodiscover-audit`**  -  Fleet table of autodiscover on/off state per tenant.

  _Surfaces tenants where new hires will silently go unprotected because autodiscover is off._

  ```bash
  skykick-cli autodiscover-audit --only-off --agent
  ```
- **`partner-rollup`**  -  Protection posture aggregated by partner for distributor oversight.

  _Gives a distributor a per-partner protection scorecard the API can't assemble._

  ```bash
  skykick-cli partner-rollup --agent
  ```

### Backup integrity
- **`stale-snapshots`**  -  Every mailbox not snapshotted within N hours, fleet-wide.

  _Silently-stale mailboxes are the top MSP backup liability; this finds them all in one call._

  ```bash
  skykick-cli stale-snapshots --hours 48 --agent
  ```
- **`coverage-gaps`**  -  Discovered-but-unprotected mailboxes and SharePoint sites per tenant.

  _Catches new hires and sites that exist but aren't actually backed up after onboarding or churn._

  ```bash
  skykick-cli coverage-gaps --type mailboxes --agent
  ```
- **`drift`**  -  Diffs two sync snapshots and reports protection-state changes since last sync.

  _Surfaces newly-stale mailboxes, dropped subscriptions, and enablement flips between reviews._

  ```bash
  skykick-cli drift --agent
  ```

### Alert ops
- **`alert-sweep`**  -  One ranked list of open alerts across the whole fleet, with optional bulk mark-complete.

  _Replaces N per-id portal lookups with one cross-fleet triage view plus bulk closure._

  ```bash
  skykick-cli alert-sweep --agent
  ```

### Async control
- **`watch-operation`**  -  Polls an async operation to a terminal state with backoff in one command.

  _Lets onboarding discovery complete unattended instead of hand-polling operation status._

  ```bash
  skykick-cli watch-operation 1a2b3c4d-0000-0000-0000-000000000000 --timeout 300
  ```

## Command Reference

**alerts**  -  Alerts for backup services and email migration orders

- `skykick-cli alerts complete`  -  Mark a specific alert as complete
- `skykick-cli alerts list`  -  List alerts for a backup service or email migration order (max 500; the API does not support skip paging)

**backup**  -  Cloud Backup subscriptions - M365 Exchange and SharePoint protection per customer tenant

- `skykick-cli backup autodiscover`  -  Auto-discover state (enabled/disabled) for Exchange and SharePoint
- `skykick-cli backup by-partner`  -  List backup subscription orders for a specific partner
- `skykick-cli backup datacenters`  -  List the Azure data centers available for backup storage
- `skykick-cli backup discover-mailboxes`  -  Trigger Exchange mailbox discovery (async; poll the returned operation with watch-operation)
- `skykick-cli backup discover-sites`  -  Trigger SharePoint site discovery (async; poll the returned operation with watch-operation)
- `skykick-cli backup jobs`  -  Active backup jobs for the subscription (known upstream defect: may return Unknown error, community-reported since 2024)
- `skykick-cli backup last-snapshot-stats`  -  Last snapshot statistics for all mailboxes in the subscription
- `skykick-cli backup list`  -  List all placed backup subscription orders across your customers
- `skykick-cli backup mailbox`  -  Details of a specific Exchange mailbox in the subscription
- `skykick-cli backup mailboxes`  -  Exchange mailboxes and their backup enabled/disabled status (IndividualMailboxes array)
- `skykick-cli backup retention-period`  -  Data retention periods for Exchange and SharePoint (response field ExchangeRentionPeriodInDays is the upstream spelling)
- `skykick-cli backup sites`  -  SharePoint site URLs and their backup enabled/disabled status
- `skykick-cli backup sku`  -  SKU and promotional details for a backup subscription
- `skykick-cli backup storage-settings`  -  Storage settings for a backup subscription
- `skykick-cli backup subscription-settings`  -  Subscription settings: Exchange/SharePoint backup state, enabled counts, customer info

**identity**  -  Authenticated caller identity

- `skykick-cli identity`  -  Show the identity and context of the authenticated API user

**operations**  -  Async operation tracking and work queue

- `skykick-cli operations status`  -  Poll the status of an async operation (e.g. a discovery run)
- `skykick-cli operations workqueue`  -  Retrieve the work queue for the authenticated account


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
skykick-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Morning fleet protection check

```bash
skykick-cli fleet-sync && skykick-cli fleet-health --flag-gaps --agent
```

Refresh the fleet store then emit one posture row per tenant with gap flags - the daily is-everyone-protected loop.

### Find silently-failing backups

```bash
skykick-cli stale-snapshots --hours 48 --agent --select mailbox,subscription_id,last_snapshot
```

Mailboxes with no snapshot in 48h, narrowed to the three fields an agent needs to open tickets.

### Post-onboarding coverage reconciliation

```bash
skykick-cli coverage-gaps --type mailboxes --agent
```

After running 'backup discover-mailboxes <subscriptionId>' and 'watch-operation <operationId>' (the discover response carries the id), list anything discovered but not protected.

### Cross-fleet alert triage

```bash
skykick-cli alert-sweep --agent
```

Fan /Alerts across every stored subscription and return one ranked open-alert list.

### QBR retention attestation

```bash
skykick-cli retention-audit --floor-days 365 --agent --select company,exchange_retention_days,status
```

Grade every tenant against a 1-year retention floor and keep only the columns the QBR deck needs.

## Auth Setup

SkyKick uses OAuth2 client-credentials behind Azure API Management with a twist: the token request needs HTTP Basic auth (API user ID + subscription key) AND an Ocp-Apim-Subscription-Key header, and every API call carries both the Bearer token and the subscription key. Set SKYKICK_CLIENT_ID to your API user ID and SKYKICK_CLIENT_SECRET to your subscription key (Partner Portal -> Settings -> User Profile -> Developer API Access; click Show on the Partner Subscription). The CLI mints and caches tokens automatically - SkyKick rate-limits the token endpoint aggressively, so cached reuse matters. Set SKYKICK_OAUTH_SCOPE=Distributor for distributor accounts (default Partner).

Run `skykick-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  skykick-cli alerts list <id> --agent --select id,name,status
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
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

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
skykick-cli feedback "the --since flag is inclusive but docs say exclusive"
skykick-cli feedback --stdin < notes.txt
skykick-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/skykick-cli/feedback.jsonl`. They are never POSTed unless `SKYKICK_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `SKYKICK_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
skykick-cli profile save briefing --json
skykick-cli --profile briefing alerts list <id>
skykick-cli profile list --json
skykick-cli profile show briefing
skykick-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `skykick-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/skykick/cmd/skykick-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add skykick-mcp -- skykick-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which skykick-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   skykick-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `skykick-cli <command> --help`.
