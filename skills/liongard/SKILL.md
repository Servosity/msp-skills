---
name: liongard
description: "Every Liongard endpoint, plus an offline copy of your whole MSP estate you can join, search, and drift-check from one command. Trigger phrases: `what changed across my Liongard environments`, `show stale Liongard launchpoints`, `which Liongard agents are offline`, `pivot a Liongard metric across all systems`, `Liongard drift report`, `use liongard`, `run liongard`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Liongard"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - liongard-cli
    install:
      - kind: go
        bins: [liongard-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/liongard/cmd/liongard-cli
---

# Liongard  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `liongard-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install liongard --cli-only
   ```
2. Verify: `liongard-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/liongard/cmd/liongard-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Liongard's API answers one environment at a time and only through the web UI or per-instance REST calls. This CLI syncs every environment, system, launchpoint, inspector, agent, detection, metric, and timeline entry into a local SQLite store, then runs cross-estate joins the live API never returns: drift since a window, stale launchpoints, offline agents, monitoring-coverage gaps, and metric pivots. Agent-native throughout  -  --json, --select, --csv, and typed exit codes.

## When to Use This CLI

Reach for this CLI when an agent needs whole-estate Liongard answers  -  what changed across all clients, which collectors are stale, which agents are offline, where monitoring coverage is missing, or one metric pivoted across every system. It is the right tool for scripted MSP health sweeps and QBR/SLA reporting where the per-environment web UI and one-call-at-a-time API are too slow. Prefer it over raw API calls whenever the question spans more than one environment or needs offline, structured, composable output.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Whole-estate visibility
- **`drift`**  -  Every change detected across all your client environments within a time window, joined to the owning environment and system.

  _Reach for this when an agent needs the whole-estate 'what changed overnight' answer instead of paging the per-environment detections endpoint._

  ```bash
  liongard-cli drift --since 24h --agent
  ```
- **`environments overview`**  -  One client's complete picture in a single command: its systems, each system's launchpoints, agent, latest inspection, open detections, and key metrics.

  _Use when an agent is asked about a specific client and needs the joined picture rather than five separate endpoint calls._

  ```bash
  liongard-cli environments overview 42 --full --agent
  ```
- **`systems history`**  -  The full chronological change history of one system: every detection and inspection entry in time order.

  _Reach for this when an agent needs 'when did this system change / break' instead of paging timeline and detections endpoints separately._

  ```bash
  liongard-cli systems history 4821 --agent
  ```

### Inspection health
- **`launchpoints stale`**  -  Every launchpoint whose newest inspection is older than a threshold, with the owning environment and system named.

  _Use to find collectors that silently stopped reporting before a client notices their data is old._

  ```bash
  liongard-cli launchpoints stale --older-than 7d --agent
  ```
- **`agents offline`**  -  Every offline agent across the estate, joined to the environment it serves.

  _Use for the daily 'is everything still collecting' health sweep across all clients at once._

  ```bash
  liongard-cli agents offline --agent
  ```
- **`coverage`**  -  Monitoring gaps: systems with no launchpoint bound, and environments with no systems at all.

  _Use during onboarding QA to catch clients you provisioned but never pointed an inspector at._

  ```bash
  liongard-cli coverage --agent
  ```
- **`launchpoints run-stale`**  -  Find stale launchpoints and trigger an inspection run on each, in one guarded command.

  _Use to re-kick every collector that fell behind, instead of clicking run on each launchpoint in the UI._

  ```bash
  liongard-cli launchpoints run-stale --older-than 7d
  ```
- **`detections failures`**  -  Every inspection that ran but failed or errored across the estate, joined to the owning environment.

  _Reach for this to find inspections that ran but errored - a different failure mode from collectors that silently stopped reporting (launchpoints stale)._

  ```bash
  liongard-cli detections failures --since 7d --agent
  ```
- **`inspectors coverage`**  -  Which environments are missing a given inspector type - the estate-wide rollout-gap view.

  _Reach for this when an agent needs 'who is missing the M365 inspector' instead of paging launchpoints per environment._

  ```bash
  liongard-cli inspectors coverage --inspector "Microsoft 365" --agent
  ```
- **`health`**  -  One estate-wide health scorecard: stale launchpoints, offline agents, failed inspections, and coverage gaps as a single summary with a typed exit code.

  _Reach for this for a single-command daily health check with a typed exit code, instead of running each sweep separately._

  ```bash
  liongard-cli health --agent
  ```

### Reporting and SLA
- **`metrics pivot`**  -  One RoarPath metric pulled across every system as a system-by-value table, CSV-ready for reports.

  _Use to assemble a single metric across the whole estate for an SLA or QBR deck in one command._

  ```bash
  liongard-cli metrics pivot "MFA Enabled Count" --csv
  ```
- **`metrics breach`**  -  Every system whose RoarPath metric value crosses a numeric threshold.

  _Use for SLA-breach and security-posture checks across all clients without N per-system calls._

  ```bash
  liongard-cli metrics breach "Patch Age Days" --op gt --value 30 --agent
  ```

## Command Reference

**access-keys**  -  Manage access keys

- `liongard-cli access-keys create`  -  Create an Access Token with the permission of user or with only 'Add Agent' permission
- `liongard-cli access-keys delete`  -  Delete Access Token created by user
- `liongard-cli access-keys get`  -  Returns a List of Access Tokens created by user
- `liongard-cli access-keys get-count`  -  Return a count of all Access Tokens created by user

**agents**  -  Agents can be installed in the Cloud or On-Premise and are responsible for running inspections.

- `liongard-cli agents delete`  -  Remove an agent
- `liongard-cli agents get`  -  List all agents.
- `liongard-cli agents get-agentid`  -  Get a specific Agent.
- `liongard-cli agents get-count`  -  Returns a count of all the agents in your service provider.
- `liongard-cli agents update`  -  Edits a deployed liongard agent, cannot update On-Demand agents.

**authentication**  -  Manage authentication

- `liongard-cli authentication log-in`  -  Returns a session token as well as additional information about logged-in user
- `liongard-cli authentication verify-token`  -  Verify token with MFA authentication code

**detections**  -  A Detection is a Change that was detected on a system.

- `liongard-cli detections detections`  -  Returns a list of all detection events.
- `liongard-cli detections detectionsby-id`  -  Gets a specific detection.
- `liongard-cli detections get-count1234`  -  Returns count of all detection events.

**environments**  -  An environment in Roar represents a single end customer for your MSP. An environment will contain Agents, Launchpoints (configured inspections), and Systems (data that has landed as a result of an inspection).

- `liongard-cli environments count`  -  Returns a count of all environments in your Service Provider.
- `liongard-cli environments create`  -  Creates a single Liongard Environment.
- `liongard-cli environments create-bulk`  -  Create many environments at once.
- `liongard-cli environments delete`  -  Delete an environment.
- `liongard-cli environments get`  -  Fetch all environments in your Service Provider.
- `liongard-cli environments get-environmentid`  -  Get a single environment.
- `liongard-cli environments update`  -  Update a single environment, If you want to update a child environment, set the Parent Environment, if not set to null

**groups**  -  A Group represents a combined set of permissions that can be assigned to Users

- `liongard-cli groups`  -  Returns a List of available Assignable Roles for a user

**inspector**  -  An Inspector represents the system type and used for building the config templates for launchpoints


**inspectors**  -  An Inspector represents the system type and used for building the config templates for launchpoints

- `liongard-cli inspectors`  -  Lists all avaialble Inspectors in Liongard

**launchpoints**  -  A Launchpoint is a configured Inspection. As an example, if you set up a domain inspector to run on liongard.com, that would be considered a Launchpoint.

- `liongard-cli launchpoints bulk-delete`  -  Remove all launchpoints.
- `liongard-cli launchpoints bulk-run`  -  Kick off many inspections.
- `liongard-cli launchpoints delete`  -  Remove a single launchpoint.
- `liongard-cli launchpoints get`  -  Lists all launchpoints.
- `liongard-cli launchpoints get-count`  -  Returns a count of all launchpoints.
- `liongard-cli launchpoints launchpoint`  -  Create a launchpoint.
- `liongard-cli launchpoints launchpointsby-id`  -  Return a specific launchpoint by ID.
- `liongard-cli launchpoints update`  -  Edit a single inspector launchpoint.
- `liongard-cli launchpoints update-bulk`  -  Update many launchpoints to run on the same schedule.

**logs**  -  Manage logs

- `liongard-cli logs`  -  Return the logs for a specific inspection.

**metrics**  -  Manage metrics

- `liongard-cli metrics create`  -  Creates a Single Metrics for a system requires valid JMESPath in the Query field
- `liongard-cli metrics delete`  -  Deletes only Custom Created Metrics, Liongard Created Metrics can not be deleted
- `liongard-cli metrics evaluation`  -  For each system ID passed
- `liongard-cli metrics evaluation-post`  -  For each system ID passed
- `liongard-cli metrics metrics`  -  Returns a list of metrics that have been created.
- `liongard-cli metrics update`  -  Updates a Metric requires valid JMESPath in the Query field

**systems**  -  A System in Roar represents a system that has been inspected by a Launchpoint. When the inspection completes for the first time, a system is completed and a corresponding timeline entry is created each time the inspection lands.

- `liongard-cli systems get-count`  -  Count of all systems in your service provider.
- `liongard-cli systems systems`  -  List all systems.

**tasks**  -  Manage tasks

- `liongard-cli tasks get-alert`  -  Returns a single alert that has been raised.
- `liongard-cli tasks get-count12345`  -  Returns a count of all alerts.
- `liongard-cli tasks list-alerts`  -  Returns a list of alerts that have been raised.

**timeline**  -  A Timeline entry represents a single inspection event. It links a System to it's configuration at that point in time.

- `liongard-cli timeline get-count123`  -  Return count of all timeline entries.
- `liongard-cli timeline timeline`  -  Fetch all timeline entries.
- `liongard-cli timeline timelineby-id`  -  Fetch a specific timeline.

**users**  -  Create and manage users in your Liongard instance

- `liongard-cli users count`  -  Returns a count of total users
- `liongard-cli users create`  -  When creating and using the Manger or Reader Roles it is required to use either the Environment/EnvironmentID or
- `liongard-cli users delete`  -  Remove a single User.
- `liongard-cli users get`  -  Returns a list of users in your Liongard Instance
- `liongard-cli users get-single`  -  Returns a Single User
- `liongard-cli users update`  -  Updates a single User


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
liongard-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Morning drift triage as JSON

```bash
liongard-cli drift --since 24h --agent --select environment,system,name
```

Whole-estate change feed for the last day, projected to just the fields an agent needs.

### Estate-wide stale collector report

```bash
liongard-cli launchpoints stale --older-than 7d --csv
```

CSV of every launchpoint that stopped reporting, ready to paste into a ticket.

### Pivot one metric across all systems

```bash
liongard-cli metrics pivot "MFA Enabled Count" --csv
```

A system-by-value table for a single RoarPath metric across the estate.

### Narrow a verbose detections payload

```bash
liongard-cli detections detections --agent --select ID,EnvironmentID,Name
```

Detections responses are large; --select keeps only the high-gravity fields so an agent does not burn context.

### Find security-posture breaches

```bash
liongard-cli metrics breach "Patch Age Days" --op gt --value 30 --agent
```

Every system whose patch age crosses the threshold, across all clients.

## Auth Setup

Liongard issues an Access Key ID and an Access Key Secret per user. The CLI sends them as the X-ROAR-API-KEY header (base64 of `accessKeyId:accessKeySecret`). Set LIONGARD_INSTANCE (your subdomain, e.g. us1), LIONGARD_ACCESS_KEY_ID, and LIONGARD_ACCESS_KEY_SECRET; or set a pre-encoded LIONGARD_API_KEY directly. Run `doctor` to confirm the host and credentials resolve.

Run `liongard-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  liongard-cli access-keys get --agent --select id,name,status
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
liongard-cli feedback "the --since flag is inclusive but docs say exclusive"
liongard-cli feedback --stdin < notes.txt
liongard-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/liongard-cli/feedback.jsonl`. They are never POSTed unless `LIONGARD_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `LIONGARD_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
liongard-cli profile save briefing --json
liongard-cli --profile briefing access-keys get
liongard-cli profile list --json
liongard-cli profile show briefing
liongard-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `liongard-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/liongard/cmd/liongard-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add liongard-mcp -- liongard-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which liongard-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   liongard-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `liongard-cli <command> --help`.
