# Resource Guru CLI

**Every Resource Guru endpoint as a typed command, plus a local database and per-resource per-day utilization no Resource Guru report gives you.**

Sync your whole schedule into local SQLite, then run offline search, SQL, and derived analytics. The headline is `utilization`  -  booked-vs-available for every resource on every day in a range  -  alongside `overbooked`, `bench`, and `capacity` planning views computed from the per-day booking breakdown the API exposes but never aggregates for you.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `resourceguru-cli` and `resourceguru-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/resourceguru/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/resourceguru/install.ps1 | iex
   ```
3. Verify: `resourceguru-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=resourceguru). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/resourceguru --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/resourceguru --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `resourceguru-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the resourceguru skill from https://github.com/Servosity/msp-skills/tree/main/skills/resourceguru. The skill defines how its required CLI (`resourceguru-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=resourceguru).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `RESOURCEGURU_EMAIL` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. A bundle carries the five platform binaries the builder downloads - macOS (`darwin-arm64`, `darwin-amd64`), Linux (`linux-arm64`, `linux-amd64`) and Windows (`windows-amd64`). Windows on ARM is released as a standalone binary but is not bundled, so use the manual config below there.

> **Interim note:** check any `.mcpb` bundle before you trust it ([#287](https://github.com/Servosity/msp-skills/issues/287)). Its `manifest.json` launches `${__dirname}/bin/resourceguru-mcp`, while the builder stores the release binaries in `bin/` under their platform-suffixed names - `resourceguru-mcp-darwin-arm64`, `-darwin-amd64`, `-linux-arm64`, `-linux-amd64`, `-windows-amd64.exe`. Run `unzip -l <file>.mcpb | grep bin/`: if the name the manifest launches is not among them, Claude Desktop has nothing to run - use the installer above and the manual JSON config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/resourceguru/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/resourceguru/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "resourceguru": {
      "command": "resourceguru-mcp",
      "env": {
        "RESOURCEGURU_EMAIL": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Resource Guru authenticates with HTTP Basic using your account email and password. Set RESOURCEGURU_EMAIL and RESOURCEGURU_PASSWORD, plus RESOURCEGURU_ACCOUNT for your account URL id (run `accounts list` to find it).

## Quick Start

```bash
# health check, confirms auth + account are configured
resourceguru-cli doctor --dry-run

# find your account URL id for RESOURCEGURU_ACCOUNT
resourceguru-cli accounts list

# pull resources, bookings, projects, clients into the local store
resourceguru-cli sync

# per-resource per-day utilization for the month
resourceguru-cli utilization --start 2026-06-01 --end 2026-06-30

```

## Unique Features

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

## Usage

Run `resourceguru-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `RESOURCEGURU_CONFIG_DIR`, `RESOURCEGURU_DATA_DIR`, `RESOURCEGURU_STATE_DIR`, or `RESOURCEGURU_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `RESOURCEGURU_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export RESOURCEGURU_HOME=/srv/resourceguru
resourceguru-cli doctor
```

Under `RESOURCEGURU_HOME=/srv/resourceguru`, the four dirs resolve to `/srv/resourceguru/config`, `/srv/resourceguru/data`, `/srv/resourceguru/state`, and `/srv/resourceguru/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

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

Precedence matters in fleets: an ambient per-kind variable such as `RESOURCEGURU_DATA_DIR` overrides an explicit `--home` for that kind. Use `RESOURCEGURU_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `RESOURCEGURU_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `resourceguru-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### accounts

Operations for retrieving accounts.

- **`resourceguru-cli accounts get`** - Returns details for the specific account by its unique identifier.
- **`resourceguru-cli accounts list`** - Returns an array of active accounts and suspended accounts.
- **`resourceguru-cli accounts update`** - Returns updated account.

### activity-types

Operations for creating, retrieving or modifying the activity types and associated entities for an account.

- **`resourceguru-cli activity-types create`** - Create a new activity_type.
- **`resourceguru-cli activity-types delete`** - Delete an activity type.
- **`resourceguru-cli activity-types get`** - Returns an array of active activity types.
- **`resourceguru-cli activity-types get-activitytypes`** - Returns an array of archived activity types.
- **`resourceguru-cli activity-types get-activitytypes-2`** - Returns a specific activity type.
- **`resourceguru-cli activity-types update`** - Update an activity type.
- **`resourceguru-cli activity-types update-activitytypes`** - Update an activity type.

### bookings

Operations for creating, retrieving or modifying the bookings for an account.

- **`resourceguru-cli bookings create`** - Create booking
- **`resourceguru-cli bookings delete`** - Delete a specific booking by its unique identifier.
- **`resourceguru-cli bookings get`** - List bookings
- **`resourceguru-cli bookings get-bookings`** - Returns a specific booking by its unique identifier.
- **`resourceguru-cli bookings update`** - Update a specific booking by its unique identifier.

### calendars

Operations for retrieving or modifying calendar subscriptions for an account.

- **`resourceguru-cli calendars delete`** - Disconnects and deletes a specific calendar by its unique identifier.
- **`resourceguru-cli calendars get`** - Lists all calendar subscriptions linked to your Resource Guru account
- **`resourceguru-cli calendars get-calendars`** - Lists external events that have been synchronised from calendar subscriptions
- **`resourceguru-cli calendars get-calendars-2`** - Returns a specific calendar by its unique identifier.
- **`resourceguru-cli calendars update`** - Update a specific calendar by its unique identifier.

### clients

Operations for creating, retrieving or modifying the clients and associated entities for an account.

- **`resourceguru-cli clients create`** - Create a new client
- **`resourceguru-cli clients delete`** - Delete a client
- **`resourceguru-cli clients get`** - Returns an array of active clients.
- **`resourceguru-cli clients get-clients`** - Returns an array of archived clients
- **`resourceguru-cli clients get-clients-2`** - Returns a specific client
- **`resourceguru-cli clients update`** - Update a client

### custom-available-periods

Manage custom available periods

- **`resourceguru-cli custom-available-periods create`** - Set custom availability for a resource
- **`resourceguru-cli custom-available-periods delete`** - Resets the custom availability for a resource on the specified date range.
- **`resourceguru-cli custom-available-periods get`** - Get custom availability for all resources or a subset of resources.

### custom-field-options

Manage custom field options

- **`resourceguru-cli custom-field-options create`** - Creates a new custom field option.
- **`resourceguru-cli custom-field-options delete`** - Delete a custom field option.
- **`resourceguru-cli custom-field-options get`** - Returns an array of custom field options.
- **`resourceguru-cli custom-field-options get-customfieldoptions`** - Returns a specific custom field option.
- **`resourceguru-cli custom-field-options update`** - Updates a specific custom field option.

### custom-fields

Operations for creating, retrieving or modifying the custom fields and associated entities for an account.

- **`resourceguru-cli custom-fields create`** - Creates a new custom field.
- **`resourceguru-cli custom-fields delete`** - Delete a custom field.
- **`resourceguru-cli custom-fields get`** - Returns an array of custom fields.
- **`resourceguru-cli custom-fields get-customfields`** - Returns a specific custom field.
- **`resourceguru-cli custom-fields update`** - Updates a specific custom field.

### downtime-types

Manage downtime types

- **`resourceguru-cli downtime-types <account>`** - Retrieve the list of downtime event types.

### downtimes

Operations for creating, retrieving or modifying the downtime events for an account.

- **`resourceguru-cli downtimes create`** - Create a new downtime event.
- **`resourceguru-cli downtimes create-downtimes`** - Gets a list of clashing booking durations for resources in a time range. This can be used to determine whether creating a downtime will result in changes to schedule bookings.
- **`resourceguru-cli downtimes delete`** - Delete a downtime event.
- **`resourceguru-cli downtimes get`** - Search for downtime events.
- **`resourceguru-cli downtimes get-downtimes`** - Get a downtime event by its unique identifier.
- **`resourceguru-cli downtimes update`** - Update a downtime event.

### me

Manage me

- **`resourceguru-cli me`** - Returns a summary about the currently authenticated user.

### overtimes

Operations for setting overtime on a resource

- **`resourceguru-cli overtimes <account>`** - Set the overtime for a resource on a specified date

### projects

Operations for creating, retrieving or modifying the projects and associated entities for an account.

- **`resourceguru-cli projects create`** - Create a new project.
- **`resourceguru-cli projects delete`** - Delete a project.
- **`resourceguru-cli projects get`** - Returns an array of active projects.
- **`resourceguru-cli projects get-projects`** - Returns an array of archived projects.
- **`resourceguru-cli projects get-projects-2`** - Returns a specific project.
- **`resourceguru-cli projects update`** - Update a project.

### rates

Operations for creating, retrieving or modifying rates.

- **`resourceguru-cli rates create`** - Create a new rate
- **`resourceguru-cli rates delete`** - Delete a rate
- **`resourceguru-cli rates get`** - List rates
- **`resourceguru-cli rates update`** - Update a specific rate by id

### reports

Operations for retrieving reporting information.

- **`resourceguru-cli reports get`** - Gets a report for all clients in the date range.
- **`resourceguru-cli reports get-reports`** - Gets a report for all projects in the date range.
- **`resourceguru-cli reports get-reports-10`** - Gets a report for a single client in the date range.
- **`resourceguru-cli reports get-reports-11`** - Gets a report on bookings not assigned to a project in the date range.
- **`resourceguru-cli reports get-reports-12`** - Gets a report for a single project in the date range.
- **`resourceguru-cli reports get-reports-13`** - Gets a report for a single resource in the date range.
- **`resourceguru-cli reports get-reports-2`** - Gets a report for all resources in the date range.
- **`resourceguru-cli reports get-reports-3`** - Gets a report for all clients in the date range.
- **`resourceguru-cli reports get-reports-4`** - Gets a report for all projects in the date range.
- **`resourceguru-cli reports get-reports-5`** - Gets a report for all resources in the date range.
- **`resourceguru-cli reports get-reports-6`** - Gets a report for the specified client in the date range.
- **`resourceguru-cli reports get-reports-7`** - Gets a report for the specified project in the date range.
- **`resourceguru-cli reports get-reports-8`** - Gets a report for a specific resource in the date range.
- **`resourceguru-cli reports get-reports-9`** - Gets a report on bookings not assigned to a client in the date range.

### resource-types

Manage resource types

- **`resourceguru-cli resource-types get`** - Returns an array of resource types
- **`resourceguru-cli resource-types get-resourcetypes`** - Returns a specific resource type

### resources

Operations for creating, retrieving or modifying the resources and associated entities for an account

- **`resourceguru-cli resources create`** - Create a new resource.  The availability will be created with the default availability settings set within the app
- **`resourceguru-cli resources delete`** - Delete a resource. Any future bookings where the resource is the booker will be transferred to the authenticated user. And any future bookings where the resource has been booked as the resource will be deleted.
- **`resourceguru-cli resources get`** - Returns an array of active resources
- **`resourceguru-cli resources get-resources`** - Returns an array of archived resources
- **`resourceguru-cli resources get-resources-2`** - Returns details of a specific resource
- **`resourceguru-cli resources update`** - Update a resource

### timesheets

Operations for creating, retrieving or modifying the timesheet entries for an account.

- **`resourceguru-cli timesheets create`** - Create a new timesheet entry
- **`resourceguru-cli timesheets create-timesheets`** - Dismiss multiple timesheet entries by their unique identifiers.
- **`resourceguru-cli timesheets get`** - List timesheet entries
- **`resourceguru-cli timesheets get-timesheets`** - Get timesheet entry reports grouped by projects
- **`resourceguru-cli timesheets get-timesheets-2`** - Get timesheet entry reports grouped by resources
- **`resourceguru-cli timesheets update`** - Update multiple timesheet entries by their unique identifiers.
- **`resourceguru-cli timesheets update-timesheets`** - Update a specific timesheet entry by its unique identifier.

### users

Operations for creating, retrieving or modifying users.

- **`resourceguru-cli users get`** - Returns all active users for this account.
- **`resourceguru-cli users get-users`** - Returns information about all active and deleted users for the account.
- **`resourceguru-cli users get-users-2`** - Returns information about the currently authenticated user in the context of an account.
- **`resourceguru-cli users get-users-3`** - Returns information about the specified user.

### webhooks

Resource Guru supports integration with other services using outgoing webhooks. In a nutshell, webhooks provide a way
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

- **`resourceguru-cli webhooks create`** - Create a new webhook
- **`resourceguru-cli webhooks create-webhooks`** - Sends a test payload to the specified URL and responds with the status code of the request
- **`resourceguru-cli webhooks delete`** - Deletes a webhook
- **`resourceguru-cli webhooks get`** - Returns a list of webhooks configured for this account
- **`resourceguru-cli webhooks get-webhooks`** - Retrieves a single webhook
- **`resourceguru-cli webhooks update`** - Updates a webhook


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
resourceguru-cli accounts list

# JSON for scripting and agents
resourceguru-cli accounts list --json

# Filter to specific fields
resourceguru-cli accounts list --json --select id,name,status

# Dry run  -  show the request without sending
resourceguru-cli accounts list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
resourceguru-cli accounts list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
resourceguru-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `resourceguru-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/resource-guru-pp-cli/config.toml`; `--home`, `RESOURCEGURU_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `RESOURCEGURU_EMAIL` | per_call | Yes |  |
| `RESOURCEGURU_PASSWORD` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `resourceguru-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `resourceguru-cli doctor` to check credentials
- Verify the environment variable is set: `echo $RESOURCEGURU_EMAIL`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call**  -  Set RESOURCEGURU_EMAIL and RESOURCEGURU_PASSWORD; Resource Guru uses HTTP Basic with your login email and password.
- **404 or empty results on account-scoped commands**  -  Set RESOURCEGURU_ACCOUNT to your account URL id from `accounts list`, or pass --account.
- **utilization shows zeros**  -  Run `sync` first; utilization reads bookings and resources from the local store.
