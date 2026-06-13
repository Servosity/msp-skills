---
name: axcient
description: "Every x360Recover endpoint, plus the fleet-wide backup-health answers the API alone can't give  -  offline, joined, and agent-ready. Trigger phrases: `check axcient backups`, `which backups failed last night`, `x360recover fleet health`, `backup compliance report for client`, `axcient usage reconciliation`, `use axcient`, `run axcient-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Axcient"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - axcient-cli
    install:
      - kind: go
        bins: [axcient-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/axcient/cmd/axcient-cli
---

# Axcient x360Recover  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `axcient-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install axcient --cli-only
   ```
2. Verify: `axcient-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/axcient/cmd/axcient-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

The only CLI and MCP surface for Axcient's x360Recover BCDR platform. It absorbs the full public API (vaults, appliances, devices, jobs, restore points, AutoVerify, usage, D2C agent tokens), then syncs everything into local SQLite so commands like 'health', 'client-rollup', and 'compliance' answer fleet-wide questions the per-entity API cannot  -  including the client-to-device correlation the raw API famously omits.

## When to Use This CLI

Reach for this CLI when an MSP task involves Axcient x360Recover backup monitoring: morning backup-health sweeps, restore-point/RPO checks, AutoVerify boot-verification evidence, per-client usage reconciliation, vault or appliance inventory, and D2C agent token minting. It is the right tool for fleet-wide questions spanning many clients, because it joins synced data locally instead of walking per-entity endpoints.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI to install, configure, or repair backup agents on endpoints  -  it only mints D2C install tokens; deployment is an RMM job.
- Do not use it to browse or restore actual backup data (files, images, VMs)  -  restores happen in the x360Recover appliance/vault UI.
- Do not use it for Axcient x360Sync or x360Cloud products  -  this covers the x360Recover public API only.
- Do not use it to manage x360Portal users or billing plans  -  the API exposes read-only user listings at most.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Fleet health that compounds locally
- **`health`**  -  See every device fleet-wide whose latest backup job failed or went stale, grouped by client, in one command.

  _Run this first for any 'which backups are broken right now' question across an MSP fleet instead of walking per-client endpoints._

  ```bash
  axcient-cli health --agent
  ```
- **`client-rollup`**  -  One row per client: devices total, failing, stale, RPO-breach, and AutoVerify-fail counts  -  the dashboard view MSPs build by hand today.

  _Reach for this when asked for a per-client backup posture summary rather than aggregating device lists yourself._

  ```bash
  axcient-cli client-rollup --agent
  ```
- **`billing`**  -  Roll up protected-system counts and storage usage for every client into one exportable table for invoice reconciliation.

  _Use this at month-end to reconcile what each client consumes against what they're invoiced, without per-client page visits._

  ```bash
  axcient-cli billing --csv
  ```
- **`appliance-map`**  -  Show which devices each appliance protects alongside each device's latest backup-job state.

  _Use this when triaging an appliance to see everything it protects and what state those backups are in._

  ```bash
  axcient-cli appliance-map --agent
  ```

### Compliance receipts
- **`compliance`**  -  Per-device compliance rows pairing newest restore-point age with AutoVerify boot-proof results and an RPO pass/fail verdict, exportable as CSV or JSON.

  _Use this to produce backup-compliance evidence for reports and audits instead of screenshotting the web UI device by device._

  ```bash
  axcient-cli compliance --client 42 --hours 24 --csv
  ```
- **`rpo`**  -  Flag devices whose newest restore point is older than a recovery-point objective threshold, grouped by client.

  _A job can succeed while RPO still slips; use this for restore-point-age questions and 'health' for job-status questions._

  ```bash
  axcient-cli rpo --hours 24 --agent
  ```

## HTTP Transport

This CLI uses Chrome-compatible HTTP transport for browser-facing endpoints. It does not require a resident browser process for normal API calls.

## Command Reference

**appliance**  -  Manage appliance

- `axcient-cli appliance get`  -  This request returns list of appliances belonging to the organization related to the user
- `axcient-cli appliance get-by-id`  -  This request returns information about appliance and assigned devices.

**client**  -  These requests are used for managing clients


**clients**  -  These requests are used for managing clients

- `axcient-cli clients get`  -  This request returns information about clients and their health.
- `axcient-cli clients get-by-id`  -  Returns the client information for organization related to the user by client ID

**device**  -  These requests are used for managing devices

- `axcient-cli device get-by-id-org-level`  -  This request returns information about device by its id.
- `axcient-cli device get-by-org-id-org-level`  -  This request returns information about all devices
- `axcient-cli device get-vault-restore-point-by-asio-endpoint-id-org-level`  -  Returns information about device's restore points grouped by Cloud Vaults it is replicated on

**organization**  -  Manage organization

- `axcient-cli organization`  -  This request returns information about organization retrieved by auth data

**user**  -  Manage user

- `axcient-cli user get-all-for-org`  -  This request returns brief information about all organization users.
- `axcient-cli user get-single-for-org`  -  Returns information about user related to organization.

**vault**  -  These requests are used for managing vaults

- `axcient-cli vault get`  -  This request returns information about vaults and assigned devices.
- `axcient-cli vault get-by-id`  -  This request returns information about vault and assigned devices.


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
axcient-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Morning fleet sweep

```bash
axcient-cli sync && axcient-cli health --agent
```

Refresh the store then list every failed/stale device grouped by client - the daily NOC triage in two commands.

### QBR compliance evidence

```bash
axcient-cli compliance --client 42 --hours 24 --csv
```

Exportable per-device rows pairing restore-point age with AutoVerify boot-proof for one client's review.

### Narrow a deep device payload

```bash
axcient-cli device get-by-org-id-org-level --agent --select id_,name,current_health_status.status
```

Device objects are deeply nested; --select keeps agent context small by returning only the fields that matter.

### Month-end billing reconciliation

```bash
axcient-cli billing --csv
```

Protected-system counts and storage per client in one table to match against invoices.

### Keyless evaluation against the public mock

```bash
axcient-cli organization --json
```

Axcient hosts a public wiremock fixture server - export AXCIENT_BASE_URL=https://ax-pub-recover.wiremockapi.cloud/x360recover (any non-empty AXCIENT_API_KEY) and the whole CLI works with no real tenant.

## Auth Setup

Authentication uses an organization-scoped API key sent as the X-Api-Key header. Generate one in the x360Portal (Settings > API Keys, admin role required) and export it as AXCIENT_API_KEY. To evaluate the CLI without any credentials, point it at Axcient's public mock server: export AXCIENT_BASE_URL=https://ax-pub-recover.wiremockapi.cloud/x360recover and any non-empty AXCIENT_API_KEY value.

Run `axcient-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  axcient-cli appliance get --agent --select id,name,status
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
axcient-cli feedback "the --since flag is inclusive but docs say exclusive"
axcient-cli feedback --stdin < notes.txt
axcient-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/axcient-cli/feedback.jsonl`. They are never POSTed unless `AXCIENT_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `AXCIENT_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
axcient-cli profile save briefing --json
axcient-cli --profile briefing appliance get
axcient-cli profile list --json
axcient-cli profile show briefing
axcient-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `axcient-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/axcient/cmd/axcient-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add axcient-mcp -- axcient-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which axcient-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   axcient-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `axcient-cli <command> --help`.
