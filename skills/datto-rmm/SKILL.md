---
name: datto-rmm
description: "Every Datto RMM API operation, plus a local SQLite fleet store and fleet-wide analytics no other Datto tool has. Trigger phrases: `datto rmm stale devices`, `which endpoints have antivirus disabled`, `datto rmm patch gaps`, `qbr scorecard for a site`, `warranty expiring devices`, `use datto-rmm`, `run datto-rmm-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Datto RMM"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - datto-rmm-cli
    install:
      - kind: go
        bins: [datto-rmm-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/datto-rmm/cmd/datto-rmm-cli
---

# Datto RMM  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `datto-rmm-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install datto-rmm --cli-only
   ```
2. Verify: `datto-rmm-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/datto-rmm/cmd/datto-rmm-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

datto-rmm-cli mirrors the full Datto RMM v2 API (sites, devices, alerts, jobs, audit, variables) and syncs your whole multi-site fleet into local SQLite. That unlocks questions no single API call can answer: stale devices, alert storms, software sprawl, warranty cliffs, patch and AV gaps, and one-shot QBR scorecards  -  all offline, agent-native, and composable with jq.

## When to Use This CLI

Use this CLI when an agent or MSP technician needs to answer fleet-wide questions about a Datto RMM account  -  which endpoints are stale, unprotected, behind on patches, or out of warranty  -  or to script bulk reads/writes (alerts, variables, quick jobs) that the web UI makes tedious. It is the right tool when the question spans more than one customer site, because the answer lives in the local store, not a single API call.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI for one-off interactive tasks better done in the Datto RMM web console (remote-control sessions, monitor policy editing, component authoring)  -  the API does not expose those surfaces.
- Do not use it for other RMM platforms (NinjaOne, N-central, ConnectWise Automate)  -  each has its own connector.
- Do not use it for Datto BCDR/backup appliances  -  that is the datto-bcdr connector, a separate API.
- Do not use fleet analytics output as live truth without running sync first  -  the local store starts empty and reflects the last sync, not the current console.
- Do not script destructive bulk writes (resetApiKeys, site moves, bulk resolve) without --dry-run/plan-only review first; fleet resolve-storm refuses to act without --confirm.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Fleet hygiene that compounds
- **`fleet stale`**  -  Lists every device across all customer sites that hasn't checked in within a chosen window, so you catch dead agents before the customer does.

  _Reach for this when you need the authoritative list of unreachable or abandoned endpoints across the entire fleet, not just one site._

  ```bash
  datto-rmm-cli fleet stale --days 30 --agent --select hostname,siteName,lastSeen,online
  ```
- **`fleet sprawl`**  -  Rolls up audited software across the whole fleet to show how many installs of an app exist and the spread of versions, exposing license waste and unpatched copies.

  _Reach for this when you need a license-true or vulnerability-true count of an application across every endpoint, not a per-device lookup._

  ```bash
  datto-rmm-cli fleet sprawl --name "Google Chrome" --agent
  ```
- **`fleet agent-drift`**  -  Shows which devices run out-of-date RMM agents across the fleet, ranked by how far behind the newest version they are.

  _Reach for this to find endpoints whose stale RMM agent may be missing monitors before troubleshooting visibility gaps._

  ```bash
  datto-rmm-cli fleet agent-drift --agent --select hostname,siteName,cagVersion,lastSeen
  ```

### Alert intelligence
- **`fleet storms`**  -  Ranks the noisiest devices and sites by alert volume over a time window so the NOC can tune monitors instead of drowning in tickets.

  _Reach for this to find the alert-fatigue offenders driving ticket volume before recommending monitor or threshold changes._

  ```bash
  datto-rmm-cli fleet storms --days 7 --top 20 --agent
  ```
- **`fleet av-gaps`**  -  Finds every device fleet-wide where antivirus is missing, disabled, or not running, so security gaps surface before an incident does.

  _Reach for this when you need the definitive list of unprotected endpoints across all customers for a security review or incident sweep._

  ```bash
  datto-rmm-cli fleet av-gaps --status not-running --agent --select hostname,siteName,antivirus.antivirusProduct,antivirus.antivirusStatus
  ```
- **`fleet resolve-storm`**  -  Bulk-resolves the open alerts on the noisiest devices and sites from the storm ranking, so Monday triage ends with a clean queue instead of a worklist.

  _Reach for this when a monitor misfire flooded the queue and you need to act on the ranking, not just look at it; defaults to a plan-only preview, and nothing resolves without --confirm._

  ```bash
  datto-rmm-cli fleet resolve-storm --days 7 --top 10 --agent
  ```
- **`fleet device-timeline`**  -  Stitches one device's alerts and activity-log entries (which surface job, audit, and user actions) into a single chronological timeline from the local store.

  _Reach for this during triage when a device keeps misbehaving and you need its full recent history in one stream, not four console tabs._

  ```bash
  datto-rmm-cli fleet device-timeline ACME-DC01 --days 30 --agent
  ```

### Lifecycle and compliance
- **`fleet patch-gaps`**  -  Ranks every device by missing-patch count across all sites so you remediate the most-exposed endpoints first.

  _Reach for this to prioritize patch remediation fleet-wide by exposure rather than checking one device at a time._

  ```bash
  datto-rmm-cli fleet patch-gaps --min-missing 5 --agent
  ```
- **`fleet warranty`**  -  Surfaces every device whose hardware warranty expires within a window, sorted by date and site, ready to drop into a QBR refresh plan.

  _Reach for this when preparing a hardware-refresh or QBR budget conversation that needs the expiring-warranty list across all customers._

  ```bash
  datto-rmm-cli fleet warranty --within 60 --agent --select hostname,siteName,warrantyDate,deviceType.type
  ```
- **`fleet scorecard`**  -  Produces a one-shot per-site health card fusing device counts, alert volume, patch coverage, AV coverage, warranty exposure, and agent drift for QBRs.

  _Reach for this when you or a vCIO need an instant customer-health summary to open a QBR or flag an at-risk account._

  ```bash
  datto-rmm-cli fleet scorecard "Acme Corporation" --agent
  ```
- **`fleet orphans`**  -  Cross-references long-stale, suspended, or deleted devices that still count against a site so you stop billing and monitoring phantom endpoints.

  _Reach for this when cleaning up a site's device roster or auditing billing accuracy against what is actually alive._

  ```bash
  datto-rmm-cli fleet orphans --stale-days 90 --agent
  ```

### Receipts and data trust
- **`fleet job-results`**  -  Fetches every device's result for one quick job and rolls them into a single pass/fail table, so a 200-device push is verified in one command.

  _Reach for this after any quick-job push to answer which devices errored or printed nothing; fetches live per device, so it needs credentials and a synced cohort (--devices, --site, or --all)._

  ```bash
  datto-rmm-cli fleet job-results 8d3f2c1a-4b6e-4f0a-9c2d-1a2b3c4d5e6f --devices 0a1b2c3d-uid1,4e5f6a7b-uid2 --failed-only --agent
  ```
- **`fleet snapshot`**  -  Freezes the current synced fleet state into a labeled, dated archive so any number you report can be reproduced later as a receipt.

  _Reach for this before a QBR or audit so a disputed number weeks later can be proven against the dated snapshot; run sync first so the receipt reflects fresh data._

  ```bash
  datto-rmm-cli fleet snapshot --label q2-acme --note "QBR baseline"
  ```
- **`fleet diff`**  -  Diffs two fleet snapshots (or a snapshot vs the live store) to show devices added or removed and warranty, patch, AV, and agent-version movement since the baseline.

  _Reach for this in QBR prep to lead with the delta story: what got better, what slipped, what arrived and left; both sides read the synced store, so sync before snapshotting._

  ```bash
  datto-rmm-cli fleet diff --from q1-acme --to q2-acme --agent
  ```
- **`fleet sync-gaps`**  -  Verifies every synced resource pulled all pages by comparing stored row counts against the API's reported totals, so you can trust fleet numbers before reporting them.

  _Reach for this after sync and before any fleet report; an incomplete devices table makes every downstream number quietly wrong._

  ```bash
  datto-rmm-cli fleet sync-gaps --agent
  ```

## Command Reference

**account**  -  Manage account

- `datto-rmm-cli account create-variable`  -  Creates an account variable
- `datto-rmm-cli account delete-variable`  -  Deletes the account variable identified by the given variable Id.
- `datto-rmm-cli account get-components`  -  Fetches the components records of the authenticated user's account.
- `datto-rmm-cli account get-dnet-site-mappings`  -  Fetches the sites records with its mapped dnet network id for the authenticated user's account.
- `datto-rmm-cli account get-sites`  -  Fetches the site records of the authenticated user's account.
- `datto-rmm-cli account get-user`  -  Fetches the authenticated user's account data.
- `datto-rmm-cli account get-user-closed-alerts`  -  If the muted parameter is not provided, both muted and umuted alerts will be queried.
- `datto-rmm-cli account get-user-devices`  -  Fetches the devices of the authenticated user's account.
- `datto-rmm-cli account get-user-open-alerts`  -  If the muted parameter is not provided, both muted and umuted alerts will be queried.
- `datto-rmm-cli account get-users`  -  Fetches the authentication users records of the authenticated user's account.
- `datto-rmm-cli account get-variables`  -  Fetches the account variables.
- `datto-rmm-cli account update-variable`  -  Updates the account variable identified by the given variable Id.

**activity-logs**  -  Manage activity logs

- `datto-rmm-cli activity-logs`  -  Fetches the activity logs.

**alert**  -  Manage alert

- `datto-rmm-cli alert <alertUid>`  -  Fetches data of the alert identified by the given alert Uid.

**audit**  -  Manage audit

- `datto-rmm-cli audit get-device`  -  Fetches audit data of the generic device identified the given device Uid.
- `datto-rmm-cli audit get-device-by-mac-address`  -  Fetches audit data of the generic device(s) identified the given MAC address in format: XXXXXXXXXXXX
- `datto-rmm-cli audit get-device-software`  -  Fetches audited software of the generic device identified the given device Uid.
- `datto-rmm-cli audit get-esxi-host`  -  Fetches audit data of the ESXi host identified the given device Uid.
- `datto-rmm-cli audit get-printer`  -  Fetches audit data of the printer identified the given device Uid.

**device**  -  Manage device

- `datto-rmm-cli device get-by-id`  -  Fetches data of the device identified by the given device Id.
- `datto-rmm-cli device get-by-mac-address`  -  Fetches data of the device(s) identified by the given MAC address in format: XXXXXXXXXXXX
- `datto-rmm-cli device get-by-uid`  -  Fetches data of the device identified by the given device Uid.

**filter**  -  Manage filter

- `datto-rmm-cli filter get-custom`  -  Fetches the custom device filters for the user (using administrator role).
- `datto-rmm-cli filter get-defaults`  -  Fetches the default device filters.

**job**  -  Manage job

- `datto-rmm-cli job <jobUid>`  -  Fetches data of the job identified by the given job Uid.

**site**  -  Manage site

- `datto-rmm-cli site create`  -  Creates a new site in the authenticated user's account.
- `datto-rmm-cli site get`  -  Fetches data of the site (including total number of devices) identified by the given site Uid.
- `datto-rmm-cli site update`  -  Updates the site identified by the given site Uid.

**system**  -  Manage system

- `datto-rmm-cli system get`  -  Fetches the request rate status for the authenticated user's account.
- `datto-rmm-cli system get-pagination-configurations`  -  Fetches the pagination configurations.
- `datto-rmm-cli system get-status`  -  Fetches the system status (start date, status and version).

**user**  -  Manage user

- `datto-rmm-cli user`  -  Resets the authenticated user's API access and secret keys.


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
datto-rmm-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Find unprotected endpoints for a security review

```bash
datto-rmm-cli fleet av-gaps --status not-running --agent --select hostname,siteName,antivirus.antivirusProduct,antivirus.antivirusStatus
```

Returns only the fields an agent needs from the deeply-nested device record (antivirus is a sub-object), so context stays small.

### Patch-remediation worklist by exposure

```bash
datto-rmm-cli fleet patch-gaps --min-missing 5 --agent
```

Every device missing 5+ patches across all sites, ranked, ready to feed a remediation job.

### QBR prep for one customer

```bash
datto-rmm-cli fleet scorecard "Acme Corporation" --agent
```

One health card fusing device counts, open alerts, patch %, AV %, warranty exposure, and agent drift.

### Hardware-refresh budget list

```bash
datto-rmm-cli fleet warranty --within 90 --agent --select hostname,siteName,warrantyDate
```

Devices whose warranty expires within 90 days, sorted by date  -  drop straight into a QBR.

## Auth Setup

Datto RMM uses OAuth 2.0. You have an API key and an API secret key (Setup > Users > generate API keys in the Datto RMM web UI). The CLI mints and auto-refreshes a bearer token for you from DATTO_RMM_API_KEY + DATTO_RMM_API_SECRET_KEY (no manual token handling). Pick your region with DATTO_RMM_PLATFORM (pinotage, merlot, concord, vidal, zinfandel, syrah) or set DATTO_RMM_API_URL directly. Tokens last ~100 hours and refresh automatically on 401.

Run `datto-rmm-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  datto-rmm-cli alert <id> --agent --select id,name,status
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
datto-rmm-cli feedback "the --since flag is inclusive but docs say exclusive"
datto-rmm-cli feedback --stdin < notes.txt
datto-rmm-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/datto-rmm-cli/feedback.jsonl`. They are never POSTed unless `DATTO_RMM_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `DATTO_RMM_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
datto-rmm-cli profile save briefing --json
datto-rmm-cli --profile briefing alert <id>
datto-rmm-cli profile list --json
datto-rmm-cli profile show briefing
datto-rmm-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `datto-rmm-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/datto-rmm/cmd/datto-rmm-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add datto-rmm-mcp -- datto-rmm-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which datto-rmm-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   datto-rmm-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `datto-rmm-cli <command> --help`.
