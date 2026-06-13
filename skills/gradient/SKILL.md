---
name: gradient
description: "The Synthesize partner API from the terminal - every endpoint, plus a usage-push ledger, billing drift detection, and alert-to-ticket tracing no other Gradient tool has. Trigger phrases: `push usage counts to gradient`, `sync usage to synthesize`, `check my synthesize integration status`, `find unmapped gradient accounts`, `did my gradient alert create a ticket`, `use gradient`, `run gradient-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Gradient MSP"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - gradient-cli
    install:
      - kind: go
        bins: [gradient-cli]
        module: github.com/mvanhorn/printing-press-library/library/payments/gradient/cmd/gradient-cli
---

# Gradient MSP  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `gradient-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install gradient --cli-only
   ```
2. Verify: `gradient-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/payments/gradient/cmd/gradient-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Gradient MSP's Synthesize platform reconciles vendor usage against MSP billing, but its only tooling is a PowerShell SDK that makes you write a script project per integration. This CLI covers the full vendor API surface agent-natively with an offline SQLite mirror of your accounts, and adds what the API itself cannot: a local ledger of every count you push (usage drift), debounce-aware bulk pushes (usage push), mapping-hygiene rollups (hygiene unmapped), and async alert-to-ticket confirmation (alert send --wait).

## When to Use This CLI

Use this CLI for any agent task that touches Gradient MSP's Synthesize vendor API: pushing usage unit counts for billing reconciliation, creating or updating accounts and services, checking or flipping integration status, dispatching alerts that must become PSA tickets, and auditing what was pushed (drift, ledgers, unmapped rollups). It is the right choice for nightly usage sync jobs and integration bring-up debugging.

## Anti-triggers

Do not use this CLI for:
- Pulling usage data OUT of vendor products (RMM, security tools) - this CLI only pushes into Synthesize; fetch from the vendor's own API first
- Operating the MSP-side Synthesize web app (mapping approvals happen in the Gradient UI, not this API)
- Managing PSA tickets directly (ConnectWise/Autotask/Halo operations) - alerts here only enqueue tickets via Gradient
- Gradient's internal partner program administration or anything requiring a Synthesize web login session

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Billing you can audit
- **`usage push`**  -  Push a whole file of unit counts (CSV or JSON) in one shot and trigger exactly one billing rebuild instead of one per call.

  _Reach for this whenever more than one unit count needs to land in Synthesize; it is the nightly sync primitive._

  ```bash
  gradient-cli usage push --file ./counts.csv --agent
  ```
- **`usage drift`**  -  See exactly which accounts' usage changed between your last two pushes, with old-to-new deltas.

  _Run before every invoice cycle to know what moved - the reconciliation pre-flight no API call can answer._

  ```bash
  gradient-cli usage drift --agent
  ```

### Alert-to-ticket correlation
- **`alert send`**  -  Dispatch an alert and wait until the PSA ticket actually exists, instead of fire-and-forget.

  _Use when an alert must verifiably become a ticket - CI hooks and monitoring bridges need the confirmation, not the enqueue._

  ```bash
  gradient-cli alert send --account 123456789 --title "Backup failure" --description "Nightly job failed" --wait --agent
  ```
- **`alert trace`**  -  Review every alert you have dispatched - messageId, account, time, last-known ticket state - and filter to ones that never became tickets.

  _The fastest answer to 'which of my alerts are stuck in the queue' across days of dispatches._

  ```bash
  gradient-cli alert trace --stuck --agent
  ```

### Mapping hygiene
- **`hygiene unmapped`**  -  One rollup of every unmapped account and every service without a vendor SKU, shaped as an actionable work queue.

  _Run it after every account/service push to see what mapping work remains before billing can reconcile._

  ```bash
  gradient-cli hygiene unmapped --agent
  ```
- **`status ready`**  -  A single go/no-go verdict on whether the integration is ready to flip to active: live status, last sync age, and outstanding mapping gaps.

  _Run before 'integration update-status --status active' to avoid activating an integration with unmapped accounts._

  ```bash
  gradient-cli status ready --agent
  ```

## Command Reference

**accounts**  -  Manage accounts

- `gradient-cli accounts create`  -  Creates an account
- `gradient-cli accounts list`  -  Get Organization Accounts
- `gradient-cli accounts update`  -  Updates 1 or more fields per Account
- `gradient-cli accounts update-one`  -  Updates 1 or more fields of a single Account

**alerting**  -  Manage alerting

- `gradient-cli alerting <accountId>`  -  Adds an alert to the alerting queue

**billing**  -  Manage billing

- `gradient-cli billing <serviceId>`  -  Set route for adding new unit count for one account, and one service.

**clients**  -  Manage clients

- `gradient-cli clients`  -  DEPRECATED - This route is used to get the Client data needed to handling mapping externally.

**integration**  -  Manage integration

- `gradient-cli integration get`  -  Preferred route for checking credentials
- `gradient-cli integration update-status`  -  Update the integration status for a partner. Set as pending when ready for inital mapping.

**mappings**  -  Manage mappings

- `gradient-cli mappings create`  -  Create Service for a Vendor for an Organization
- `gradient-cli mappings update`  -  Updates 1 or more fields of a single Service
- `gradient-cli mappings update-bulk`  -  Updates 1 or more fields for each Service

**services**  -  Manage services

- `gradient-cli services create`  -  Create Service for a Vendor
- `gradient-cli services get <serviceId>`  -  Retrieves Vendor with VendorSku[] filtered by service id

**ticket_events**  -  Manage ticket events

- `gradient-cli ticket-events <messageId>`  -  Checks for a ticket Event with the provided messageId to have created a ticket in the PSA

**vendor**  -  Manage vendor

- `gradient-cli vendor get`  -  Get Vendor Details
- `gradient-cli vendor update`  -  Update details for a vendor


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
gradient-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Nightly usage sync with one billing rebuild

```bash
gradient-cli usage push --file ./nightly-counts.csv --agent
```

Validates and fans out every account-service count with no_build until the final call, then records the run in the local ledger.

### What changed since the last push?

```bash
gradient-cli usage drift --agent
```

Joins the two most recent ledger runs and returns only the rows whose counts moved - the pre-invoice sanity check.

### Inspect the vendor SKU catalog fields agents care about

```bash
gradient-cli vendor get --agent --select data.name,data.skus.name,data.skus.category
```

The vendor profile nests the full SKU catalog; --select narrows the payload to the fields that matter.

### Dispatch an alert and confirm the ticket

```bash
gradient-cli alert send --account 123456789 --title "Backup failure" --description "Nightly job failed twice" --wait
```

Dispatches, captures the messageId, then polls the debug route until the PSA ticket exists or timeout.

### Pre-activation readiness check

```bash
gradient-cli status ready --agent
```

Combines live integration status with local mapping gaps into a single go/no-go before 'integration update-status --status active'.

## Auth Setup

Synthesize issues a Vendor API key and a Partner API key pair (Synthesize UI: Integrations -> Custom -> Generate API Tokens; shown once). Every request sends a GRADIENT-TOKEN header whose value is base64("<vendorKey>:<partnerKey>"). Set GRADIENT_VENDOR_API_KEY and GRADIENT_PARTNER_API_KEY and the CLI derives the header for you, or set GRADIENT_TOKEN to the pre-computed value directly. Verify with `gradient-cli doctor`.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  gradient-cli accounts list --agent --select id,name,status
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
gradient-cli feedback "the --since flag is inclusive but docs say exclusive"
gradient-cli feedback --stdin < notes.txt
gradient-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/gradient-cli/feedback.jsonl`. They are never POSTed unless `GRADIENT_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `GRADIENT_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
gradient-cli profile save briefing --json
gradient-cli --profile briefing accounts list
gradient-cli profile list --json
gradient-cli profile show briefing
gradient-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `gradient-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/payments/gradient/cmd/gradient-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add gradient-mcp -- gradient-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which gradient-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   gradient-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `gradient-cli <command> --help`.
