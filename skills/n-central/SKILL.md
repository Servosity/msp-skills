---
name: n-central
description: "Every N-central REST endpoint, plus an offline SQLite mirror of your whole org tree, cross-tenant search, issue-triage rollups, and a JWT-expiry guardian no other N-central tool has."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "N-able N-central"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - n-central-cli
    install:
      - kind: go
        bins: [n-central-cli]
        module: github.com/mvanhorn/printing-press-library/library/developer-tools/n-central/cmd/n-central-cli
---

# N Central  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `n-central-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install n-central --cli-only
   ```
2. Verify: `n-central-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/developer-tools/n-central/cmd/n-central-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

N-able N-central RMM REST API  -  manage devices, customers, sites, org units, active issues, custom properties, scheduled tasks, and maintenance windows across an MSP's N-central instance.

## Command Reference

**access-groups**  -  Access groups (device-type and org-unit-type).

- `n-central-cli access-groups <accessGroupId>`  -  Retrieve detailed information for an access group by ID.

**customers**  -  Customers (client organizations) in N-central.

- `n-central-cli customers get`  -  Retrieve a single customer by ID.
- `n-central-cli customers list`  -  List all customers across the instance.
- `n-central-cli customers registration-token`  -  Retrieve the agent registration token for a customer (used to enroll new devices).

**device-filters**  -  Saved device filters (reusable as filterId on device list calls).

- `n-central-cli device-filters`  -  List saved device filters for the API user.

**devices**  -  Devices monitored by N-central (workstations, servers, network devices, probes).

- `n-central-cli devices assets`  -  Retrieve hardware/software asset inventory for a device.
- `n-central-cli devices get`  -  Retrieve a single device by ID.
- `n-central-cli devices list`  -  List all devices visible to the API user, across the org tree.
- `n-central-cli devices maintenance`  -  List patch maintenance windows configured for a device.
- `n-central-cli devices properties`  -  List custom property values for a device (the backbone of MSP automation/documentation).
- `n-central-cli devices status`  -  Retrieve the service-monitoring status (active issues / health) for a device.
- `n-central-cli devices tasks`  -  List scheduled/automation tasks targeting this device.

**org-units**  -  Organization units  -  the unified tree of service orgs, customers, and sites.

- `n-central-cli org-units access-groups`  -  List access groups for an org unit.
- `n-central-cli org-units active-issues`  -  Fetch active monitoring issues for an org unit (the daily NOC triage feed).
- `n-central-cli org-units children`  -  List the direct children of an org unit.
- `n-central-cli org-units devices`  -  List devices scoped to a specific org unit.
- `n-central-cli org-units get`  -  Retrieve a single org unit by ID.
- `n-central-cli org-units job-statuses`  -  Fetch job statuses for an org unit.
- `n-central-cli org-units list`  -  List all organization units (SO, customer, and site nodes).
- `n-central-cli org-units registration-token`  -  Retrieve the agent registration token for an org unit.
- `n-central-cli org-units user-roles`  -  List user roles defined for an org unit.

**scheduled-tasks**  -  Scheduled tasks  -  run scripts/automation policies on devices and track them.

- `n-central-cli scheduled-tasks get`  -  Retrieve general information for a scheduled task.
- `n-central-cli scheduled-tasks run`  -  Create a direct-support scheduled task (run an Automation Policy, Script, or MacScript on a device).
- `n-central-cli scheduled-tasks status`  -  Retrieve aggregated status for a scheduled task.

**server**  -  Server info and health.

- `n-central-cli server health`  -  Return the start and current time of the server (lightweight reachability check).
- `n-central-cli server info`  -  Return version information for the N-central API service and systems.

**service-orgs**  -  Service Organizations  -  the top level of the N-central org tree.

- `n-central-cli service-orgs customers`  -  List all customers under a service organization.
- `n-central-cli service-orgs get`  -  Retrieve a single service organization by ID.
- `n-central-cli service-orgs list`  -  List all service organizations.

**sites**  -  Sites  -  the leaf org-unit level under customers.

- `n-central-cli sites get`  -  Retrieve a single site by ID.
- `n-central-cli sites list`  -  List all sites across the instance.

**users**  -  N-central users.

- `n-central-cli users`  -  List N-central users.


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
n-central-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Auth Setup

Run `n-central-cli auth setup` for the URL and steps to obtain a token (add `--launch` to open the URL). Then store it:

```bash
n-central-cli auth set-token YOUR_TOKEN_HERE
```

Or set `NCENTRAL_JWT` as an environment variable.

Run `n-central-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  n-central-cli access-groups <id> --agent --select id,name,status
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
n-central-cli feedback "the --since flag is inclusive but docs say exclusive"
n-central-cli feedback --stdin < notes.txt
n-central-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/n-central-cli/feedback.jsonl`. They are never POSTed unless `N_CENTRAL_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `N_CENTRAL_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
n-central-cli profile save briefing --json
n-central-cli --profile briefing access-groups <id>
n-central-cli profile list --json
n-central-cli profile show briefing
n-central-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Async Jobs

For endpoints that submit long-running work, the generator detects the submit-then-poll pattern (a `job_id`/`task_id`/`operation_id` field in the response plus a sibling status endpoint) and wires up three extra flags on the submitting command:

| Flag | Purpose |
|------|---------|
| `--wait` | Block until the job reaches a terminal status instead of returning the job ID immediately |
| `--wait-timeout` | Maximum wait duration (default 10m, 0 means no timeout) |
| `--wait-interval` | Initial poll interval (default 2s; grows with exponential backoff up to 30s) |

Use async submission without `--wait` when you want to fire-and-forget; use `--wait` when you want one command to return the finished artifact.

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

1. **Empty, `help`, or `--help`** → show `n-central-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/developer-tools/n-central/cmd/n-central-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add n-central-mcp -- n-central-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which n-central-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   n-central-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `n-central-cli <command> --help`.
