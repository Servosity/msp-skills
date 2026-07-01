---
name: resourceguru
description: "Use when the user asks to see who is overbooked in Resource Guru, find who is on the bench, check a resource's day-by-day utilization, work out remaining capacity before booking a project, or report what changed on the schedule. Syncs your whole Resource Guru schedule into a local SQLite mirror so it computes the per-day, fleet-wide utilization the reports never expose. Trigger phrases: `resource utilization by day`, `who is overbooked in resource guru`, `who is on the bench`, `remaining capacity in resource guru`, `use resource guru`, `run resourceguru-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Resource Guru"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - resourceguru-cli
    install:
      - kind: go
        bins: [resourceguru-cli]
        module: github.com/mvanhorn/printing-press-library/library/project-management/resourceguru/cmd/resourceguru-cli
---

# Resource Guru  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `resourceguru-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install resourceguru --cli-only
   ```
2. Verify: `resourceguru-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/project-management/resourceguru/cmd/resourceguru-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Sync your whole schedule into local SQLite, then run offline search, SQL, and derived analytics. The headline is `utilization`  -  booked-vs-available for every resource on every day in a range  -  alongside `overbooked`, `bench`, and `capacity` planning views computed from the per-day booking breakdown the API exposes but never aggregates for you.

## When to Use This CLI

Use this CLI when you manage a Resource Guru schedule and need answers the web reports don't give: day-level utilization per person, fleet-wide overbooking, who is on the bench, and remaining capacity for new work. Ideal for agents doing capacity planning or staffing analysis over the synced schedule.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI to run payroll or invoicing  -  it reports time, it is not a billing system.
- Do not use it as the source of truth for editing the schedule UI; writes go through the same API the web app uses, so confirm in Resource Guru after mutations.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Utilization intelligence
- **`utilization`**  -  See each resource's booked-vs-available utilization for every day in a date range, not just a range average.

  _Reach for this when you need day-level workload per person, not a blurred range average._

  ```bash
  resourceguru-cli utilization --start 2026-06-01 --end 2026-06-30 --agent
  ```
- **`overbooked`**  -  List every resource-day where booked minutes exceed available capacity across the whole fleet.

  _Use before sprint or week planning to catch overcommitted people the booking UI won't flag in aggregate._

  ```bash
  resourceguru-cli overbooked --start 2026-06-01 --end 2026-06-30 --agent
  ```
- **`bench`**  -  Find resources running below a utilization threshold in a window  -  who is free or under-used.

  _Use when you need to staff new work and want to see who has slack first._

  ```bash
  resourceguru-cli bench --start 2026-06-01 --end 2026-06-07 --threshold 50 --agent
  ```
- **`capacity`**  -  Show remaining bookable minutes per resource over a future window (capacity minus committed bookings).

  _Use to answer 'can this new project fit?' before you create the bookings._

  ```bash
  resourceguru-cli capacity --start 2026-07-01 --end 2026-07-31 --agent
  ```

### Local state that compounds
- **`since`**  -  Surface bookings created or updated within a recent window from the local store.

  _Use to catch what moved on the schedule since you last looked._

  ```bash
  resourceguru-cli since 7d --agent
  ```

## Command Reference

**accounts**  -  Operations for retrieving accounts.

- `resourceguru-cli accounts get`  -  Returns details for the specific account by its unique identifier.
- `resourceguru-cli accounts list`  -  Returns an array of active accounts and suspended accounts.
- `resourceguru-cli accounts update`  -  Returns updated account.

**activity-types**  -  Operations for creating, retrieving or modifying the activity types and associated entities for an account.

- `resourceguru-cli activity-types create`  -  Create a new activity_type.
- `resourceguru-cli activity-types delete`  -  Delete an activity type.
- `resourceguru-cli activity-types get`  -  Returns an array of active activity types.
- `resourceguru-cli activity-types get-activitytypes`  -  Returns an array of archived activity types.
- `resourceguru-cli activity-types get-activitytypes-2`  -  Returns a specific activity type.
- `resourceguru-cli activity-types update`  -  Update an activity type.
- `resourceguru-cli activity-types update-activitytypes`  -  Update an activity type.

**bookings**  -  Operations for creating, retrieving or modifying the bookings for an account.

- `resourceguru-cli bookings create`  -  Create booking
- `resourceguru-cli bookings delete`  -  Delete a specific booking by its unique identifier.
- `resourceguru-cli bookings get`  -  List bookings
- `resourceguru-cli bookings get-bookings`  -  Returns a specific booking by its unique identifier.
- `resourceguru-cli bookings update`  -  Update a specific booking by its unique identifier.

**calendars**  -  Operations for retrieving or modifying calendar subscriptions for an account.

- `resourceguru-cli calendars delete`  -  Disconnects and deletes a specific calendar by its unique identifier.
- `resourceguru-cli calendars get`  -  Lists all calendar subscriptions linked to your Resource Guru account
- `resourceguru-cli calendars get-calendars`  -  Lists external events that have been synchronised from calendar subscriptions
- `resourceguru-cli calendars get-calendars-2`  -  Returns a specific calendar by its unique identifier.
- `resourceguru-cli calendars update`  -  Update a specific calendar by its unique identifier.

**clients**  -  Operations for creating, retrieving or modifying the clients and associated entities for an account.

- `resourceguru-cli clients create`  -  Create a new client
- `resourceguru-cli clients delete`  -  Delete a client
- `resourceguru-cli clients get`  -  Returns an array of active clients.
- `resourceguru-cli clients get-clients`  -  Returns an array of archived clients
- `resourceguru-cli clients get-clients-2`  -  Returns a specific client
- `resourceguru-cli clients update`  -  Update a client

**custom-available-periods**  -  Manage custom available periods

- `resourceguru-cli custom-available-periods create`  -  Set custom availability for a resource
- `resourceguru-cli custom-available-periods delete`  -  Resets the custom availability for a resource on the specified date range.
- `resourceguru-cli custom-available-periods get`  -  Get custom availability for all resources or a subset of resources.

**custom-field-options**  -  Manage custom field options

- `resourceguru-cli custom-field-options create`  -  Creates a new custom field option.
- `resourceguru-cli custom-field-options delete`  -  Delete a custom field option.
- `resourceguru-cli custom-field-options get`  -  Returns an array of custom field options.
- `resourceguru-cli custom-field-options get-customfieldoptions`  -  Returns a specific custom field option.
- `resourceguru-cli custom-field-options update`  -  Updates a specific custom field option.

**custom-fields**  -  Operations for creating, retrieving or modifying the custom fields and associated entities for an account.

- `resourceguru-cli custom-fields create`  -  Creates a new custom field.
- `resourceguru-cli custom-fields delete`  -  Delete a custom field.
- `resourceguru-cli custom-fields get`  -  Returns an array of custom fields.
- `resourceguru-cli custom-fields get-customfields`  -  Returns a specific custom field.
- `resourceguru-cli custom-fields update`  -  Updates a specific custom field.

**downtime-types**  -  Manage downtime types

- `resourceguru-cli downtime-types <account>`  -  Retrieve the list of downtime event types.

**downtimes**  -  Operations for creating, retrieving or modifying the downtime events for an account.

- `resourceguru-cli downtimes create`  -  Create a new downtime event.
- `resourceguru-cli downtimes create-downtimes`  -  Gets a list of clashing booking durations for resources in a time range.
- `resourceguru-cli downtimes delete`  -  Delete a downtime event.
- `resourceguru-cli downtimes get`  -  Search for downtime events.
- `resourceguru-cli downtimes get-downtimes`  -  Get a downtime event by its unique identifier.
- `resourceguru-cli downtimes update`  -  Update a downtime event.

**me**  -  Manage me

- `resourceguru-cli me`  -  Returns a summary about the currently authenticated user.

**overtimes**  -  Operations for setting overtime on a resource

- `resourceguru-cli overtimes <account>`  -  Set the overtime for a resource on a specified date

**projects**  -  Operations for creating, retrieving or modifying the projects and associated entities for an account.

- `resourceguru-cli projects create`  -  Create a new project.
- `resourceguru-cli projects delete`  -  Delete a project.
- `resourceguru-cli projects get`  -  Returns an array of active projects.
- `resourceguru-cli projects get-projects`  -  Returns an array of archived projects.
- `resourceguru-cli projects get-projects-2`  -  Returns a specific project.
- `resourceguru-cli projects update`  -  Update a project.

**rates**  -  Operations for creating, retrieving or modifying rates.

- `resourceguru-cli rates create`  -  Create a new rate
- `resourceguru-cli rates delete`  -  Delete a rate
- `resourceguru-cli rates get`  -  List rates
- `resourceguru-cli rates update`  -  Update a specific rate by id

**reports**  -  Operations for retrieving reporting information.

- `resourceguru-cli reports get`  -  Gets a report for all clients in the date range.
- `resourceguru-cli reports get-reports`  -  Gets a report for all projects in the date range.
- `resourceguru-cli reports get-reports-10`  -  Gets a report for a single client in the date range.
- `resourceguru-cli reports get-reports-11`  -  Gets a report on bookings not assigned to a project in the date range.
- `resourceguru-cli reports get-reports-12`  -  Gets a report for a single project in the date range.
- `resourceguru-cli reports get-reports-13`  -  Gets a report for a single resource in the date range.
- `resourceguru-cli reports get-reports-2`  -  Gets a report for all resources in the date range.
- `resourceguru-cli reports get-reports-3`  -  Gets a report for all clients in the date range.
- `resourceguru-cli reports get-reports-4`  -  Gets a report for all projects in the date range.
- `resourceguru-cli reports get-reports-5`  -  Gets a report for all resources in the date range.
- `resourceguru-cli reports get-reports-6`  -  Gets a report for the specified client in the date range.
- `resourceguru-cli reports get-reports-7`  -  Gets a report for the specified project in the date range.
- `resourceguru-cli reports get-reports-8`  -  Gets a report for a specific resource in the date range.
- `resourceguru-cli reports get-reports-9`  -  Gets a report on bookings not assigned to a client in the date range.

**resource-types**  -  Manage resource types

- `resourceguru-cli resource-types get`  -  Returns an array of resource types
- `resourceguru-cli resource-types get-resourcetypes`  -  Returns a specific resource type

**resources**  -  Operations for creating, retrieving or modifying the resources and associated entities for an account

- `resourceguru-cli resources create`  -  Create a new resource. The availability will be created with the default availability settings set within the app
- `resourceguru-cli resources delete`  -  Delete a resource. Any future bookings where the resource is the booker will be transferred to the authenticated user.
- `resourceguru-cli resources get`  -  Returns an array of active resources
- `resourceguru-cli resources get-resources`  -  Returns an array of archived resources
- `resourceguru-cli resources get-resources-2`  -  Returns details of a specific resource
- `resourceguru-cli resources update`  -  Update a resource

**timesheets**  -  Operations for creating, retrieving or modifying the timesheet entries for an account.

- `resourceguru-cli timesheets create`  -  Create a new timesheet entry
- `resourceguru-cli timesheets create-timesheets`  -  Dismiss multiple timesheet entries by their unique identifiers.
- `resourceguru-cli timesheets get`  -  List timesheet entries
- `resourceguru-cli timesheets get-timesheets`  -  Get timesheet entry reports grouped by projects
- `resourceguru-cli timesheets get-timesheets-2`  -  Get timesheet entry reports grouped by resources
- `resourceguru-cli timesheets update`  -  Update multiple timesheet entries by their unique identifiers.
- `resourceguru-cli timesheets update-timesheets`  -  Update a specific timesheet entry by its unique identifier.

**users**  -  Operations for creating, retrieving or modifying users.

- `resourceguru-cli users get`  -  Returns all active users for this account.
- `resourceguru-cli users get-users`  -  Returns information about all active and deleted users for the account.
- `resourceguru-cli users get-users-2`  -  Returns information about the currently authenticated user in the context of an account.
- `resourceguru-cli users get-users-3`  -  Returns information about the specified user.

**webhooks**  -  Resource Guru supports integration with other services using outgoing webhooks. In a nutshell, webhooks provide a way
for Resource Guru to send real-time information to other apps. For example, when a booking is made in Resource Guru,
webhooks can be used to post information (payloads) about that booking to a payload (receiving) URL. Getting this
information was always possible via our basic API, but webhooks proactively post the changes instead. This means that
apps no longer need to keep polling the API to check what's changed - resulting in much greater efficiency.

Account owners and users with administrative privileges can create new webhooks either via the API endpoint or via
settings in their Resource Guru account. Simply specify the name of the webhook, the payload URL which receives the
payloads, and the types of events which should be sent to the payload URL. For added security, you can provide a secret
string which will be combined with the payload's request body to create a HMAC SHA256 digest and added as a request
header.

The supported event types are:

- Bookings
- Clients
- Projects
- Resources
- Resource Types
- Accounts
- Time Off
- Custom fields
- Custom field options
- Timesheet Entries
- Timesheet Submissions

As soon as changes are made within a relevant Resource Guru account, payloads are sent immediately for any of the events
that have been subscribed to in the webhook. We will automatically try to deliver a payload 100 times before marking it
as failed. More detail on payload statuses can be found in the [payloads endpoint documentation](#tag/webhook/paths/~1v1~1{account}~1webhooks~1{id}~1queue/get).
Payloads are dropped from Resource Guru's history after 30 days. **Unsuccessful payloads will be lost after failing for
30 days**.

Payloads are sent as JSON with the following headers:

| Header                   | Description                                                                                                           |
| ------------------------ | --------------------------------------------------------------------------------------------------------------------- |
| User-Agent               | The string `ResourceGuru/Webhooks` identifies Resource Guru as the sender.                                            |
| Content-Type             | The string `application/json` identifies the content type of the payload.                                             |
| X-ResourceGuru-Key       | The secret provided when creating the webhook. This is only sent if a webhook secret is set.                          |
| X-ResourceGuru-Signature | A HMAC SHA256 digest of the request body, signed by the webhook secret. This is only sent if a webhook secret is set. |

The signature is generated on our side using the OpenSSL library using the following code:

```ruby
OpenSSL::HMAC.hexdigest(OpenSSL::Digest.new('sha256'), webhook_secret, request_body)
```

## Payload format

Payloads are sent as JSON.

| Key       | Type    | Description                                                                                                                                                                                                             |
| --------- | ------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| id        | integer | Each payload has a unique incrementing ID                                                                                                                                                                               |
| timestamp | integer | A UNIX epoch timestamp when the event occurred.                                                                                                                                                                         |
| payload   | object  | Format varies based on the type of event. We use a stripped down version of whatever `type` the payload is representing, any additional information can be fetched via the API. The keys `action` and `type` are added. |

The payload `action` will be one of:

- create
- update
- delete

The payload `type` will be one of:

- account
- booking
- client
- project
- resource
- resource_type
- downtime (Time Off)
- timesheet_entry
- timesheet_submission

An example payload when a new client is created:

```js
{
  "id": 1,
  "timestamp": 1423472753,
  "payload": {
    "id": 1234,
    "archived": false,
    "color": null,
    "name": "A client",
    "notes": "Some notes",
    "created_at": "2015-02-04T16:40:23.000Z",
    "updated_at":"2015-02-09T09:05:53.581Z",
    "action": "create",
    "type": "client"
  }
}
```

- `resourceguru-cli webhooks create`  -  Create a new webhook
- `resourceguru-cli webhooks create-webhooks`  -  Sends a test payload to the specified URL and responds with the status code of the request
- `resourceguru-cli webhooks delete`  -  Deletes a webhook
- `resourceguru-cli webhooks get`  -  Returns a list of webhooks configured for this account
- `resourceguru-cli webhooks get-webhooks`  -  Retrieves a single webhook
- `resourceguru-cli webhooks update`  -  Updates a webhook


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
resourceguru-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Month utilization for one person

```bash
resourceguru-cli utilization --start 2026-06-01 --end 2026-06-30 --resource 12345 --agent
```

Per-day booked-vs-capacity for a single resource.

### Find overcommitted people this week

```bash
resourceguru-cli overbooked --start 2026-06-01 --end 2026-06-07 --agent
```

Resource-days where bookings exceed daily capacity.

### Narrow a verbose booking list

```bash
resourceguru-cli bookings get <account> --start-date 2026-06-01 --end-date 2026-06-30 --agent --select id,resource_id,start_date,end_date,durations.date,durations.duration
```

Bookings carry deep per-day duration arrays; select only the fields you need to keep agent context small. (List bookings is `bookings get <account>`; pass your account URL id.)

### Who has slack next week

```bash
resourceguru-cli bench --start 2026-06-08 --end 2026-06-14 --threshold 60 --agent
```

Resources under 60% utilization in the window.

## Auth Setup

Resource Guru authenticates with HTTP Basic using your account email and password. Set RESOURCEGURU_EMAIL and RESOURCEGURU_PASSWORD, plus RESOURCEGURU_ACCOUNT for your account URL id (run `accounts list` to find it).

Run `resourceguru-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  resourceguru-cli accounts list --agent --select id,name,status
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
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

- Use `--home <dir>` for one invocation, or set `RESOURCEGURU_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `RESOURCEGURU_CONFIG_DIR`, `RESOURCEGURU_DATA_DIR`, `RESOURCEGURU_STATE_DIR`, `RESOURCEGURU_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `RESOURCEGURU_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `resourceguru-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "resourceguru": {
        "command": "resourceguru-mcp",
        "env": {
          "RESOURCEGURU_HOME": "/srv/resourceguru"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `RESOURCEGURU_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `RESOURCEGURU_HOME`, or `doctor` will not find credentials left under the former root.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
resourceguru-cli feedback "the --since flag is inclusive but docs say exclusive"
resourceguru-cli feedback --stdin < notes.txt
resourceguru-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `RESOURCEGURU_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `RESOURCEGURU_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
resourceguru-cli profile save briefing --json
resourceguru-cli --profile briefing accounts list
resourceguru-cli profile list --json
resourceguru-cli profile show briefing
resourceguru-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `resourceguru-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/project-management/resourceguru/cmd/resourceguru-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add resourceguru-mcp -- resourceguru-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which resourceguru-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   resourceguru-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `resourceguru-cli <command> --help`.
