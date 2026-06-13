---
name: betterstack
description: "Every Better Stack Uptime feature, plus an offline SQLite mirror and cross-resource fleet analytics  -  what's down and who's paged, coverage gaps, MTTA/MTTR, flapping, on-call gaps, and status-page drift  -  that the API alone can't answer. Trigger phrases: `which monitors are down`, `find unprotected monitors`, `incident MTTR report`, `who is on call`, `noisy monitors`, `use betterstack`, `run betterstack-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Better Stack"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - betterstack-cli
    install:
      - kind: go
        bins: [betterstack-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/betterstack/cmd/betterstack-cli
---

# Better Stack  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `betterstack-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install betterstack --cli-only
   ```
2. Verify: `betterstack-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/betterstack/cmd/betterstack-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

betterstack-cli mirrors your whole Better Stack Uptime account (monitors, heartbeats, incidents, on-call, escalation policies, status pages) into a local SQLite store, then answers operational questions no single API call can: `coverage` finds monitors that won't page anyone, `mttr` rolls up incident response times, `flapping` ranks your noisiest monitors, and `fleet` shows the whole account on one screen. Covers the official Terraform provider's resource surface (full create/update/delete on monitors and heartbeats; create/delete on groups, policies, and status pages), with agent-native output and typed exit codes throughout.

## When to Use This CLI

Use this CLI when an agent or operator needs to query, audit, or provision a Better Stack Uptime account from the terminal  -  especially for cross-resource questions (which monitors are unprotected, how fast incidents resolve, which monitors flap, who is on call) that the one-resource-at-a-time API can't answer directly. Prefer it over raw API calls whenever the answer spans more than one resource type or needs to work offline.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Cross-resource analytics the API can't answer
- **`fleet`**  -  One-screen health of the whole Better Stack account: monitors up/down/paused, heartbeats, open incidents, and who's on call now.

  _Reach for this first when an agent needs the operational state of the entire account in one call instead of five._

  ```bash
  betterstack-cli fleet --agent
  ```
- **`coverage`**  -  Find monitors with no escalation policy or no alert channel  -  the ones that will go down silently and page no one.

  _Use before an incident to prove every monitor actually escalates to a human._

  ```bash
  betterstack-cli coverage --agent
  ```
- **`down`**  -  Focused triage list of monitors currently down or degraded, joined to their open incidents and whether anyone is actually paged for each.

  _Reach for this at the start of a shift or during an outage to see what is down and who is (or is not) being paged, in one call._

  ```bash
  betterstack-cli down --agent
  ```
- **`triage`**  -  Open incidents ranked by age and acknowledgement state  -  never-acknowledged first  -  joined to the affected monitor.

  _Use this to prioritize incident response: it surfaces the open incidents nobody has acknowledged yet, oldest first._

  ```bash
  betterstack-cli triage --agent
  ```
- **`statuspage-audit`**  -  Flags status pages showing operational while a backing monitor has an open incident, and status-page resources pointing at missing or paused monitors.

  _Use this before or during an incident to catch public status pages that are silently out of sync with reality._

  ```bash
  betterstack-cli statuspage-audit --agent
  ```
- **`group-health`**  -  Per-group health rollup: monitor and heartbeat up/down counts plus open incidents for every monitor group and heartbeat group.

  _For MSP-style accounts where one group is one client, this answers per-client health in a single call._

  ```bash
  betterstack-cli group-health --agent
  ```

### Incident intelligence
- **`mttr`**  -  Mean time to acknowledge and resolve, computed across incidents over a window and broken down by monitor.

  _Use for on-call retros and SLA reporting without exporting to a spreadsheet._

  ```bash
  betterstack-cli mttr --days 30 --agent
  ```
- **`flapping`**  -  Rank monitors by how many incidents they generated in a window to surface the noisy, flapping, or misconfigured ones.

  _Use to find alert fatigue sources before tuning thresholds._

  ```bash
  betterstack-cli flapping --days 7 --top 10 --agent
  ```

### On-call and resilience
- **`oncall-gaps`**  -  Detect on-call calendars with nobody currently on call.

  _Use to confirm someone is actually reachable on every rotation before relying on paging._

  ```bash
  betterstack-cli oncall-gaps --agent
  ```
- **`heartbeat-risk`**  -  Rank heartbeats by risk: tight period+grace windows, paused-but-expected, and non-up status.

  _Use to catch fragile cron/scheduled-task check-ins before they false-alarm or silently miss._

  ```bash
  betterstack-cli heartbeat-risk --agent
  ```

## Command Reference

**heartbeat-groups**  -  Heartbeat groups

- `betterstack-cli heartbeat-groups create`  -  Create a heartbeat group
- `betterstack-cli heartbeat-groups delete`  -  Delete a heartbeat group
- `betterstack-cli heartbeat-groups get`  -  Get a heartbeat group by ID
- `betterstack-cli heartbeat-groups list`  -  List heartbeat groups

**heartbeats**  -  Heartbeats  -  cron/scheduled-task check-ins

- `betterstack-cli heartbeats create`  -  Create a heartbeat
- `betterstack-cli heartbeats delete`  -  Delete a heartbeat
- `betterstack-cli heartbeats get`  -  Get a heartbeat by ID
- `betterstack-cli heartbeats list`  -  List heartbeats
- `betterstack-cli heartbeats update`  -  Update a heartbeat (sends PATCH)

**incidents**  -  Incidents  -  outages and alerts across monitors and heartbeats

- `betterstack-cli incidents acknowledge`  -  Acknowledge an incident
- `betterstack-cli incidents delete`  -  Delete an incident
- `betterstack-cli incidents get`  -  Get an incident by ID
- `betterstack-cli incidents list`  -  List incidents
- `betterstack-cli incidents resolve`  -  Resolve an incident

**monitor-groups**  -  Monitor groups

- `betterstack-cli monitor-groups create`  -  Create a monitor group
- `betterstack-cli monitor-groups delete`  -  Delete a monitor group
- `betterstack-cli monitor-groups get`  -  Get a monitor group by ID
- `betterstack-cli monitor-groups list`  -  List monitor groups

**monitors**  -  Uptime monitors (HTTP, keyword, ping, TCP, heartbeat-backed)

- `betterstack-cli monitors create`  -  Create a monitor
- `betterstack-cli monitors delete`  -  Delete a monitor
- `betterstack-cli monitors get`  -  Get a monitor by ID
- `betterstack-cli monitors list`  -  List all monitors
- `betterstack-cli monitors update`  -  Update a monitor (sends PATCH)

**on-calls**  -  On-call calendars and current on-call shifts

- `betterstack-cli on-calls get`  -  Get an on-call calendar by ID (includes current on-call users)
- `betterstack-cli on-calls list`  -  List on-call calendars

**policies**  -  Escalation policies

- `betterstack-cli policies create`  -  Create an escalation policy
- `betterstack-cli policies delete`  -  Delete an escalation policy
- `betterstack-cli policies get`  -  Get an escalation policy by ID
- `betterstack-cli policies list`  -  List escalation policies

**status-page-resources**  -  Resources (monitored components) shown on a status page

- `betterstack-cli status-page-resources delete`  -  Remove a resource from a status page
- `betterstack-cli status-page-resources get`  -  Get a status page resource
- `betterstack-cli status-page-resources list`  -  List resources on a status page

**status-page-sections**  -  Sections within a status page

- `betterstack-cli status-page-sections create`  -  Create a status page section
- `betterstack-cli status-page-sections delete`  -  Delete a status page section
- `betterstack-cli status-page-sections get`  -  Get a status page section
- `betterstack-cli status-page-sections list`  -  List sections of a status page

**status-pages**  -  Public status pages

- `betterstack-cli status-pages create`  -  Create a status page
- `betterstack-cli status-pages delete`  -  Delete a status page
- `betterstack-cli status-pages get`  -  Get a status page by ID
- `betterstack-cli status-pages list`  -  List status pages


## Freshness Contract

This printed CLI owns bounded freshness only for registered store-backed read command paths. In `--data-source auto` mode, those paths check `sync_state` and may run a bounded refresh before reading local data. `--data-source local` never refreshes. `--data-source live` reads the API and does not mutate the local store. Set `BETTERSTACK_NO_AUTO_REFRESH=1` to skip the freshness hook without changing source selection.

Covered paths:

- `betterstack-cli coverage`
- `betterstack-cli down`
- `betterstack-cli flapping`
- `betterstack-cli fleet`
- `betterstack-cli group-health`
- `betterstack-cli heartbeat-groups`
- `betterstack-cli heartbeat-groups get`
- `betterstack-cli heartbeat-groups list`
- `betterstack-cli heartbeat-risk`
- `betterstack-cli heartbeats`
- `betterstack-cli heartbeats get`
- `betterstack-cli heartbeats list`
- `betterstack-cli incidents`
- `betterstack-cli incidents get`
- `betterstack-cli incidents list`
- `betterstack-cli monitor-groups`
- `betterstack-cli monitor-groups get`
- `betterstack-cli monitor-groups list`
- `betterstack-cli monitors`
- `betterstack-cli monitors get`
- `betterstack-cli monitors list`
- `betterstack-cli mttr`
- `betterstack-cli on-calls`
- `betterstack-cli on-calls get`
- `betterstack-cli on-calls list`
- `betterstack-cli oncall-gaps`
- `betterstack-cli policies`
- `betterstack-cli policies get`
- `betterstack-cli policies list`
- `betterstack-cli status-pages`
- `betterstack-cli status-pages get`
- `betterstack-cli status-pages list`
- `betterstack-cli statuspage-audit`
- `betterstack-cli triage`

When JSON output uses the generated provenance envelope, freshness metadata appears at `meta.freshness`. Treat it as current-cache freshness for the covered command path, not a guarantee of complete historical backfill or API-specific enrichment.

### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
betterstack-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Find unprotected monitors

```bash
betterstack-cli coverage --agent
```

Lists monitors with no escalation policy or alert channel so you can fix paging gaps before an outage.

### 30-day incident response report

```bash
betterstack-cli mttr --days 30 --agent
```

Computes MTTA and MTTR across the window from the local mirror  -  no spreadsheet export needed.

### Narrow a verbose monitor payload

```bash
betterstack-cli monitors list --per-page 250 --agent --select data.id,data.attributes.pronounceable_name,data.attributes.status
```

Uses --select dotted paths to pull only id, name, and status from the JSON:API envelope so agents don't burn context on full monitor objects.

### Acknowledge an incident safely

```bash
betterstack-cli incidents acknowledge 12345 --by oncall@example.com --dry-run
```

Shows the exact request without sending; drop --dry-run to actually acknowledge.

### Rank the noisiest monitors

```bash
betterstack-cli flapping --days 7 --top 10 --agent
```

Surfaces the monitors generating the most incidents so you can tune alert thresholds.

## Auth Setup

Auth is a Better Stack Uptime API token (Authorization: Bearer). Create one under Better Stack → Settings → API tokens, then set BETTERSTACK_API_TOKEN. Run `betterstack-cli doctor` to confirm the token and reachability.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  betterstack-cli heartbeat-groups list --agent --select id,name,status
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
betterstack-cli feedback "the --since flag is inclusive but docs say exclusive"
betterstack-cli feedback --stdin < notes.txt
betterstack-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/betterstack-cli/feedback.jsonl`. They are never POSTed unless `BETTERSTACK_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `BETTERSTACK_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
betterstack-cli profile save briefing --json
betterstack-cli --profile briefing heartbeat-groups list
betterstack-cli profile list --json
betterstack-cli profile show briefing
betterstack-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `betterstack-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/betterstack/cmd/betterstack-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add betterstack-mcp -- betterstack-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which betterstack-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   betterstack-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `betterstack-cli <command> --help`.
