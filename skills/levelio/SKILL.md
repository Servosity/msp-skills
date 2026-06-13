---
name: levelio
description: "Every Level RMM endpoint, plus a local SQLite fleet store and offline cross-entity rollups no Level tool has: at-risk ranking, patch posture, alert triage, and stale-device detection in one command. Trigger phrases: `check my level fleet`, `what level devices are at risk`, `level patch posture`, `level stale devices`, `level client scorecard`, `triage level alerts`, `use levelio`, `run levelio-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Level"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - levelio-cli
    install:
      - kind: go
        bins: [levelio-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/levelio/cmd/levelio-cli
---

# Level  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `levelio-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install levelio --cli-only
   ```
2. Verify: `levelio-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/levelio/cmd/levelio-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

levelio-cli syncs your entire Level estate  -  devices, groups, tags, custom fields, alerts, and OS updates  -  into a local SQLite database, then answers portfolio-wide questions offline that the Level web UI shows one device at a time. Match every API operation with agent-native output (--json/--select/--csv), then transcend with weighted at-risk ranking, fleet-wide patch posture, per-client posture scorecards, group-clustered alert triage, reboot-debt tracking, and custom-field coverage audits.

## When to Use This CLI

Use levelio-cli when an agent or technician needs a portfolio-wide answer about a Level-managed fleet  -  at-risk endpoints, patch exposure, alert clusters, stale agents, security-score distribution, or custom-field gaps  -  rather than a single device's detail. It is the right tool for cross-entity rollups, anti-joins (what is missing), and scriptable RMM automation, all offline against a synced local store.

## Anti-triggers

Do not use this CLI for:
- single-device detail lookups (use devices show <device-id>, not the fleet analytics commands)
- real-time monitoring without a prior sync  -  analytics commands read the local store
- authoring automations, scripts, or monitoring policies  -  the public API only triggers automation webhooks

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Cross-entity fleet intelligence
- **`at-risk`**  -  Rank the worst endpoints across every axis at once  -  active alerts, pending patches, low security score, and how long they have been dark  -  as a single weighted risk score.

  _Reach for this when an agent needs the single prioritized 'fix these first' list across the whole fleet instead of one health axis at a time._

  ```bash
  levelio-cli at-risk --top 20 --agent
  ```
- **`patch-posture`**  -  Aggregate OS updates across the fleet  -  available vs installed, by category and device, including patches that errored  -  so you can see exposure at a glance.

  _Use when an agent must report fleet-wide patch exposure or pick which category of updates to push next._

  ```bash
  levelio-cli patch-posture --category security --agent
  ```
- **`fleet`**  -  One-screen inventory rollup, cross-tabbed any way you slice it  -  by OS, platform, group, or tag  -  with online and maintenance counts.

  _Reach for this for a portfolio-wide inventory answer instead of paging through device lists._

  ```bash
  levelio-cli fleet --by os --online --agent
  ```
- **`alert-triage`**  -  Cluster unresolved alerts by group and severity with device context, so systemic fires surface above one-off noise.

  _Reach for this to answer 'where are my fires and which are systemic' in one call._

  ```bash
  levelio-cli alert-triage --severity critical --group-by group --agent
  ```
- **`client-scorecard`**  -  One row per top-level group (client) with device count, online %, open critical alerts, average security score, stale count, and patch exposure  -  the QBR-ready per-client rollup.

  _Reach for this when an agent needs per-client fleet posture in one table  -  which client is worst, where to focus  -  instead of walking groups and devices one call at a time._

  ```bash
  levelio-cli client-scorecard --agent
  ```

### Health and drift
- **`stale`**  -  List devices that have gone dark  -  not seen in N days  -  with an option to exclude machines intentionally in maintenance mode.

  _Use to find agents that silently stopped checking in without chasing each device in the UI._

  ```bash
  levelio-cli stale --days 14 --exclude-maintenance --agent
  ```
- **`group-tree`**  -  Render the Level group hierarchy with each node showing its real rolled-up health  -  descendant device, alert, stale, and score counts.

  _Use to see the org structure annotated with where the problems actually concentrate._

  ```bash
  levelio-cli group-tree --with alerts,stale,score --agent
  ```
- **`since`**  -  Show what changed in a recent time window  -  new alerts, newly published updates, and devices last seen  -  so you can answer 'what happened since I last looked'.

  _Reach for this at the start of a shift to catch up on everything that moved overnight._

  ```bash
  levelio-cli since --hours 24 --agent
  ```
- **`alert-recurrence`**  -  Rank which alert names fire most often across the fleet and on how many distinct devices, so chronically noisy monitors surface above one-off fires.

  _Reach for this when the question is which monitors are chronically noisy and worth tuning, not which fires are burning right now (use alert-triage for that)._

  ```bash
  levelio-cli alert-recurrence --top 15 --agent
  ```
- **`reboot-due`**  -  List devices waiting on a reboot to finalize installed patches  -  and how long they have been waiting  -  joined with online and maintenance state.

  _Reach for this when patches are installed but not yet effective  -  the reboot backlog is the gap between patch posture on paper and in reality._

  ```bash
  levelio-cli reboot-due --days 3 --agent
  ```

### Governance and hygiene
- **`cf-coverage`**  -  Audit which devices, groups, or the org are missing a custom-field value  -  the absence the UI can't show  -  via an anti-join.

  _Reach for this when an agent must enforce data hygiene  -  e.g. every device must carry an asset-tag or warranty field._

  ```bash
  levelio-cli cf-coverage --missing --agent
  ```
- **`security-posture`**  -  Show the fleet security-score distribution and everyone under a threshold, optionally rolled up by group.

  _Use to report fleet security posture or target the weakest endpoints for remediation._

  ```bash
  levelio-cli security-posture --below 70 --by-group --agent
  ```
- **`tag-audit`**  -  Surface tag-data drift: devices with zero tags, orphan tags applied to nothing, and duplicate tag names that fragment the fleet.

  _Reach for this when fleet filters and automations misbehave because tag data drifted  -  untagged devices and orphan tags are invisible until audited._

  ```bash
  levelio-cli tag-audit --agent
  ```

## Command Reference

**alerts**  -  Alerts generated by monitoring devices.

- `levelio-cli alerts list`  -  Returns a list of your alerts.
- `levelio-cli alerts show`  -  Retrieves the details of an existing alert.

**automations**  -  Operations on automations.

- `levelio-cli automations <token>`  -  Triggers an automation via a webhook.

**custom-field-values**  -  Values assigned to custom fields for the organization, groups, and devices.

- `levelio-cli custom-field-values delete`  -  Deletes a custom field value for the organization, group, or device.
- `levelio-cli custom-field-values list`  -  Returns a list of custom field values for the organization, groups, or devices.
- `levelio-cli custom-field-values update`  -  Set a custom field value for the organization, group, or device.

**custom-fields**  -  Custom fields that can have values assigned to them.

- `levelio-cli custom-fields create`  -  Creates a custom field.
- `levelio-cli custom-fields delete`  -  Deletes a custom field.
- `levelio-cli custom-fields list`  -  Returns a list of custom fields.
- `levelio-cli custom-fields show`  -  Retrieves the details of an existing custom field.
- `levelio-cli custom-fields update`  -  Updates a custom field.

**devices**  -  Devices with the Level agent installed.

- `levelio-cli devices delete`  -  Deletes the specified device.
- `levelio-cli devices list`  -  Returns a list of your devices.
- `levelio-cli devices show`  -  Retrieves the details of an existing device.
- `levelio-cli devices update`  -  Updates the specified device.

**groups**  -  The group hierachy that contains devices.

- `levelio-cli groups create`  -  Creates a new group.
- `levelio-cli groups delete`  -  Deletes an existing group.
- `levelio-cli groups list`  -  Returns a list of your groups.
- `levelio-cli groups show`  -  Retrieves the details of an existing group.
- `levelio-cli groups update`  -  Updates an existing group.

**tags**  -  Tags that can be applied to devices.

- `levelio-cli tags create`  -  Creates a new tag.
- `levelio-cli tags delete`  -  Deletes a tag.
- `levelio-cli tags list`  -  Returns a list of tags.
- `levelio-cli tags show`  -  Retrieves a tag.
- `levelio-cli tags update`  -  Updates an existing tag.

**updates**  -  Update operations and status for devices.

- `levelio-cli updates list`  -  Returns a list of your updates.
- `levelio-cli updates show`  -  Retrieves the details of an existing update.


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
levelio-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Morning fleet catch-up

```bash
levelio-cli since --hours 12 --agent
```

Everything that changed overnight  -  new alerts, new updates, devices that checked in  -  in one structured payload.

### Patch-day exposure report

```bash
levelio-cli patch-posture --category security --agent --select summary,by_category
```

Fleet-wide pending vs installed security updates, narrowed to just the rollup fields an agent needs.

### Find the worst endpoints fast

```bash
levelio-cli at-risk --top 10 --agent --select rank,hostname,risk_score,reasons
```

Top-10 risk ranking with only the decision fields, ready to pipe into a ticket.

### Dark-agent sweep

```bash
levelio-cli stale --days 7 --exclude-maintenance
```

Devices that have not checked in for a week and are not in maintenance  -  the silent-failure list.

### Data-hygiene audit

```bash
levelio-cli cf-coverage --missing --agent
```

Every assignee missing a custom-field value, so required asset/warranty data can be backfilled.

### Per-client QBR rollup

```bash
levelio-cli client-scorecard --agent
```

One row per client group  -  devices, online %, open criticals, average security score, stale count, and patch exposure  -  ready for a QBR slide.

## Auth Setup

Authenticate with a Level API key (Settings -> API keys; read-only is enough for every list/show/sync/analytics command). Export it as LEVEL_API_TOKEN; it is sent as 'Authorization: Bearer <token>'.

Run `levelio-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  levelio-cli alerts list --agent --select id,name,status
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
levelio-cli feedback "the --since flag is inclusive but docs say exclusive"
levelio-cli feedback --stdin < notes.txt
levelio-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/levelio-cli/feedback.jsonl`. They are never POSTed unless `LEVELIO_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `LEVELIO_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
levelio-cli profile save briefing --json
levelio-cli --profile briefing alerts list
levelio-cli profile list --json
levelio-cli profile show briefing
levelio-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `levelio-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/levelio/cmd/levelio-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add levelio-mcp -- levelio-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which levelio-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   levelio-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `levelio-cli <command> --help`.
