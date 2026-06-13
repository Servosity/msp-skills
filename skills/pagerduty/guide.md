# PagerDuty CLI

**Every PagerDuty incident, on-call and service operation from the terminal, plus a local SQLite mirror that answers cross-entity questions  -  MTTA/MTTR, on-call coverage gaps, responder load  -  that neither the API nor the web UI can.**

Triage the incident queue, resolve who's on call now and next, and run service and escalation hygiene checks without leaving the shell. Sync once and the local store powers analytics no single API call exposes: pulse for what's hot right now, oncall who for the live escalation chain, audit coverage for escalation gaps, and insights mttr/responders/noisy for offline post-incident analytics.

Learn more at [PagerDuty](http://www.pagerduty.com/support).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

The recommended path installs both the `pagerduty-cli` binary and the `pp-pagerduty` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install pagerduty
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install pagerduty --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install pagerduty --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install pagerduty --agent claude-code
npx -y @mvanhorn/printing-press-library install pagerduty --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/pagerduty/cmd/pagerduty-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/pagerduty-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install pagerduty --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-pagerduty --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-pagerduty --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install pagerduty --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/pagerduty-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `PAGERDUTY_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/pagerduty/cmd/pagerduty-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "pagerduty": {
      "command": "pagerduty-mcp",
      "env": {
        "PAGERDUTY_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Authenticate with a PagerDuty REST API key in the Authorization header as `Token token=<key>`. Set it via the PAGERDUTY_API_KEY environment variable. Read-only by default  -  mutating commands require explicit flags and support --dry-run.

## Quick Start

```bash
# confirm the API key is set and the API is reachable
pagerduty-cli doctor

# mirror incidents, services, schedules, oncalls and log entries into the local store
pagerduty-cli sync

# see what's hot right now: open incidents by service with unacked age
pagerduty-cli pulse

# list the currently triggered incidents straight from the API
pagerduty-cli incidents list --statuses triggered --limit 20

# resolve who is on call now and next for a service
pagerduty-cli oncall who --service PXXXXXX

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Local-store analytics that compounds
- **`pulse`**  -  One offline call shows what's hot right now: open incidents bucketed by service, urgency and status with how long each has gone unacknowledged, sorted by SLA risk.

  _Reach for this instead of N web-UI tabs when an agent or NOC analyst needs the current incident picture across every service in one shot._

  ```bash
  pagerduty-cli pulse --agent
  ```
- **`audit coverage`**  -  Flags services whose escalation chain is broken: empty tiers, single point of failure, expired or empty schedules, or no escalation policy at all.

  _Run before an on-call rotation to catch services that would page nobody when they break._

  ```bash
  pagerduty-cli audit coverage --agent
  ```
- **`insights mttr`**  -  Mean time to acknowledge and resolve, computed from synced log-entry timestamps and grouped by service, team or priority.

  _Use for post-incident reviews to get MTTA/MTTR by service this month without scripting the analytics API._

  ```bash
  pagerduty-cli insights mttr --by service --since 30d --agent
  ```
- **`insights responders`**  -  Per-responder page, ack and resolve counts plus the share of pages that landed off-hours (nights and weekends).

  _Reach for this for on-call fairness and burnout reviews to see who is carrying the off-hours load._

  ```bash
  pagerduty-cli insights responders --since 30d --agent
  ```
- **`insights noisy`**  -  Ranks services by incident volume, auto-resolve rate and re-trigger/flapping rate over a window.

  _Use to find which services to tune first when alert fatigue is high._

  ```bash
  pagerduty-cli insights noisy --top 10 --since 7d --agent
  ```
- **`incidents changes`**  -  For an incident, see the change events that shipped on the same service in the window right before it triggered, ranked by proximity.

  _Reach for this first in root-cause triage: it answers 'what shipped right before this broke' without opening the web UI._

  ```bash
  pagerduty-cli incidents changes PT4KHLK --window 120m --agent
  ```
- **`audit schedule-gaps`**  -  Find future time windows where a schedule has nobody on call, before an incident finds the hole for you.

  _Run before each rotation change to catch uncovered windows; neither the API nor the web UI has an equivalent report._

  ```bash
  pagerduty-cli audit schedule-gaps --days 14 --agent
  ```
- **`insights stale`**  -  Open incidents with no log activity past a threshold, grouped by responder and service  -  the ones quietly rotting.

  _Use during shift handoffs to sweep forgotten incidents that pulse's SLA-risk ranking may not surface._

  ```bash
  pagerduty-cli insights stale --hours 24 --agent
  ```

### On-call intelligence
- **`oncall who`**  -  Resolves who is on call right now for a service or team, who is on next, and the exact handoff timestamp.

  _Use at 2am to know exactly who to escalate to and when the current responder hands off, without clicking through the UI._

  ```bash
  pagerduty-cli oncall who --service PXXXXXX --agent
  ```
- **`oncall hours`**  -  On-call hours per user over a time window, derived from synced schedule layers and overrides.

  _Reach for this for monthly on-call fairness or MSP billing reviews without the paid analytics product._

  ```bash
  pagerduty-cli oncall hours --since 30d --agent
  ```
- **`incidents timeline`**  -  Reconstructs one incident's full chronology  -  trigger, every ack, note, reassignment, escalation and resolve  -  with elapsed deltas between events.

  _Use during or after an incident to get a clean, ordered story of what happened and how long each step took._

  ```bash
  pagerduty-cli incidents timeline PXXXXXX --agent
  ```

## Recipes


### Triage the morning queue

```bash
pagerduty-cli pulse --agent
```

One offline call buckets open incidents by service and urgency with unacked age, sorted by SLA risk.

### Narrow a verbose incident list for an agent

```bash
pagerduty-cli incidents list --agent --select incidents.id,incidents.title,incidents.status,incidents.service.summary
```

PagerDuty incident objects are deeply nested; dotted --select returns only the fields the agent needs and saves context.

### Find escalation coverage gaps

```bash
pagerduty-cli audit coverage --agent
```

Flags services that would page nobody  -  empty tiers, single point of failure, expired schedules, or no policy.

### Post-incident MTTR by service

```bash
pagerduty-cli insights mttr --by service --since 30d --agent
```

Reconstructs mean time to acknowledge/resolve offline from synced log entries, grouped by service.

### Resolve who to escalate to

```bash
pagerduty-cli oncall who --service PXXXXXX --agent
```

Joins escalation policy, schedule and overrides to show who is on now, who is next, and the handoff time.

## Usage

Run `pagerduty-cli --help` for the full command reference and flag list.

## Commands

### abilities

This describes your account's abilities by feature name. For example `"teams"`.
An ability may be available to your account based on things like your pricing plan or account state.

- **`pagerduty-cli abilities get-ability`** - Test whether your account has a given ability.

"Abilities" describes your account's capabilities by feature name. For example `"teams"`.

An ability may be available to your account based on things like your pricing plan or account state.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `abilities.read`
- **`pagerduty-cli abilities list`** - List all of your account's abilities, by name.

"Abilities" describes your account's capabilities by feature name. For example `"teams"`.

An ability may be available to your account based on things like your pricing plan or account state.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `abilities.read`

### addons

Manage addons

- **`pagerduty-cli addons create`** - Install an Add-on for your account.

Addon's are pieces of functionality that developers can write to insert new functionality into PagerDuty's UI.

Given a configuration containing a `src` parameter, that URL will be embedded in an `iframe` on a page that's available to users from a drop-down menu.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `addons.write`
- **`pagerduty-cli addons delete`** - Remove an existing Add-on.

Addon's are pieces of functionality that developers can write to insert new functionality into PagerDuty's UI.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `addons.write`
- **`pagerduty-cli addons get`** - Get details about an existing Add-on.

Addon's are pieces of functionality that developers can write to insert new functionality into PagerDuty's UI.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `addons.read`
- **`pagerduty-cli addons list`** - List all of the Add-ons installed on your account.

Addon's are pieces of functionality that developers can write to insert new functionality into PagerDuty's UI.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `addons.read`
- **`pagerduty-cli addons update`** - Update an existing Add-on.

Addon's are pieces of functionality that developers can write to insert new functionality into PagerDuty's UI.

Given a configuration containing a `src` parameter, that URL will be embedded in an `iframe` on a page that's available to users from a drop-down menu.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `addons.write`

### alert-grouping-settings

Alert Grouping Settings allow you to configure how alerts in services are grouped together into incidents.

- **`pagerduty-cli alert-grouping-settings delete`** - Delete an existing Alert Grouping Setting.

The settings part of Alert Grouper service allows us to create Alert Grouping Settings and configs that are required to be used during grouping of the alerts.

Scoped OAuth requires: `services.write`
- **`pagerduty-cli alert-grouping-settings get`** - Get an existing Alert Grouping Setting.

The settings part of Alert Grouper service allows us to create Alert Grouping Settings and configs that are required to be used during grouping of the alerts.

Scoped OAuth requires: `services.read`
- **`pagerduty-cli alert-grouping-settings list`** - List all of your alert grouping settings including both single service settings and global content based settings.

The settings part of Alert Grouper service allows us to create Alert Grouping Settings and configs that are required to be used during grouping of the alerts.

Scoped OAuth requires: `services.read`
- **`pagerduty-cli alert-grouping-settings post`** - Create a new Alert Grouping Setting.

The settings part of Alert Grouper service allows us to create Alert Grouping Settings and configs that are required to be used during grouping of the alerts.

This endpoint will be used to create an instance of AlertGroupingSettings for either one service or many services that are in the alert group setting.

Scoped OAuth requires: `services.write`
- **`pagerduty-cli alert-grouping-settings put`** - Update an Alert Grouping Setting.

The settings part of Alert Grouper service allows us to create Alert Grouping Settings and configs that are required to be used during grouping of the alerts.

if `services` are not provided in the request, then the existing services will not be removed from the setting.

Scoped OAuth requires: `services.write`

### audit

Provides audit record data.

- **`pagerduty-cli audit`** - List audit trail records matching provided query params or default criteria.

The returned records are sorted by the `execution_time` from newest to oldest.

See [`Cursor-based pagination`](https://developer.pagerduty.com/docs/rest-api-v2/pagination/) for instructions on how to paginate through the result set.

Only admins, account owners, or global API tokens on PagerDuty account [pricing plans](https://www.pagerduty.com/pricing) with the "Audit Trail" feature can access this endpoint.

For other role based access to audit records by resource ID, see the resource's API documentation.

For more information see the [Audit API Document](https://developer.pagerduty.com/docs/rest-api-v2/audit-records-api/).

Scoped OAuth requires: `audit_records.read`

### automation-actions

Automation Actions invoke jobs that are staged in Runbook Automation or Process Automation.

- **`pagerduty-cli automation-actions create`** - Create a Script, Process Automation, or Runbook Automation action
- **`pagerduty-cli automation-actions create-invocation`** - Create an Invocation
- **`pagerduty-cli automation-actions create-runner`** - Create a Process Automation or a Runbook Automation runner.
- **`pagerduty-cli automation-actions create-runner-team-association`** - Associate a runner with a team
- **`pagerduty-cli automation-actions create-service-assocation`** - Associate an Automation Action with a service
- **`pagerduty-cli automation-actions create-team-association`** - Associate an Automation Action with a team
- **`pagerduty-cli automation-actions delete`** - Delete an Automation Action
- **`pagerduty-cli automation-actions delete-runner`** - Delete an Automation Action runner
- **`pagerduty-cli automation-actions delete-runner-team-association`** - Disassociates a runner from a team
- **`pagerduty-cli automation-actions delete-service-association`** - Disassociate an Automation Action from a service
- **`pagerduty-cli automation-actions delete-team-association`** - Disassociate an Automation Action from a team
- **`pagerduty-cli automation-actions get`** - Get an Automation Action
- **`pagerduty-cli automation-actions get-action-service-association`** - Gets the details of a Automation Action / service relation
- **`pagerduty-cli automation-actions get-action-service-associations`** - Gets all service references associated with an Automation Action
- **`pagerduty-cli automation-actions get-action-team-association`** - Gets the details of an Automation Action / team relation
- **`pagerduty-cli automation-actions get-action-team-associations`** - Gets all team references associated with an Automation Action
- **`pagerduty-cli automation-actions get-all`** - Lists Automation Actions matching provided query params.

The returned records are sorted by action name in alphabetical order.

See [`Cursor-based pagination`](https://developer.pagerduty.com/docs/rest-api-v2/pagination/) for instructions on how to paginate through the result set.
- **`pagerduty-cli automation-actions get-invocation`** - Get an Automation Action Invocation
- **`pagerduty-cli automation-actions get-runner`** - Get an Automation Action runner
- **`pagerduty-cli automation-actions get-runner-team-association`** - Gets the details of a runner / team relation
- **`pagerduty-cli automation-actions get-runner-team-associations`** - Gets all team references associated with a runner
- **`pagerduty-cli automation-actions get-runners`** - Lists Automation Action runners matching provided query params.
The returned records are sorted by runner name in alphabetical order.

See [`Cursor-based pagination`](https://developer.pagerduty.com/docs/rest-api-v2/pagination/) for instructions on how to paginate through the result set.
- **`pagerduty-cli automation-actions list-invocations`** - List Invocations
- **`pagerduty-cli automation-actions update`** - Updates an Automation Action
- **`pagerduty-cli automation-actions update-runner`** - Update an Automation Action runner

### business-services

Business services model capabilities that span multiple technical services and that may be owned by several different teams.

- **`pagerduty-cli business-services create`** - Create a new Business Service.

Business services model capabilities that span multiple technical services and that may be owned by several different teams.

There is a limit of 5,000 business services per account. If the limit is reached, the API will respond with an error.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.write`
- **`pagerduty-cli business-services delete`** - Delete an existing Business Service.

Once the service is deleted, it will not be accessible from the web UI and new incidents won't be able to be created for this service.

Business services model capabilities that span multiple technical services and that may be owned by several different teams.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.write`
- **`pagerduty-cli business-services delete-priority-thresholds`** - Clears the Priority Threshold for the account.  If the priority threshold is cleared, any Incident with a Priority set will be able to impact Business Services.
Scoped OAuth requires: `services.write`
- **`pagerduty-cli business-services get`** - Get details about an existing Business Service.

Business services model capabilities that span multiple technical services and that may be owned by several different teams.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.read`
- **`pagerduty-cli business-services get-impacts`** - Retrieve a list top-level Business Services sorted by highest Impact with `status` included.
When called without the `ids[]` parameter, this endpoint does not return an exhaustive list of Business Services but rather provides access to the most impacted up to the limit of 200.

The returned Business Services are sorted first by Impact, secondarily by most recently impacted, and finally by name.

To get impact information about a specific set of Business Services, use the `ids[]` parameter.
Scoped OAuth requires: `services.read`
- **`pagerduty-cli business-services get-priority-thresholds`** - Retrieves the priority threshold information for an account.  Currently, there is a `global_threshold` that can be set for the account.  Incidents that have a priority meeting or exceeding this threshold will be considered impacting on any Business Service that depends on the Service to which the Incident belongs.
Scoped OAuth requires: `services.read`
- **`pagerduty-cli business-services get-top-level-impactors`** - Retrieve a list of Impactors for the top-level Business Services on the account. Impactors are currently limited to Incidents.

This endpoint does not return an exhaustive list of Impactors but rather provides access to the highest priority Impactors for the Business Services in question up to the limit of 200.

To get Impactors for a specific set of Business Services, use the `ids[]` parameter.

The returned Impactors are sorted first by priority and secondarily by their creation date.
Scoped OAuth requires: `services.read`
- **`pagerduty-cli business-services list`** - List existing Business Services.

Business services model capabilities that span multiple technical services and that may be owned by several different teams.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.read`
- **`pagerduty-cli business-services put-priority-thresholds`** - Set the Account-level priority threshold for Business Service.
Scoped OAuth requires: `services.write`
- **`pagerduty-cli business-services update`** - Update an existing Business Service. NOTE that this endpoint also accepts the PATCH verb.

Business services model capabilities that span multiple technical services and that may be owned by several different teams.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.write`

### change-events

Change Events enable you to send informational events about recent changes such as code deploys and system config changes from any system that can make an outbound HTTP connection. These events do not create incidents and do not send notifications; they are shown in context with incidents on the same PagerDuty service.

- **`pagerduty-cli change-events create`** - Sending Change Events is documented as part of the V2 Events API. See [`Send Change Event`](https://developer.pagerduty.com/api-reference/b3A6Mjc0ODI2Ng-send-change-events-to-the-pager-duty-events-api).
- **`pagerduty-cli change-events get`** - Get details about an existing Change Event.

Scoped OAuth requires: `change_events.read`
- **`pagerduty-cli change-events list`** - List all of the existing Change Events.

Scoped OAuth requires: `change_events.read`
- **`pagerduty-cli change-events update`** - Update an existing Change Event

Scoped OAuth requires: `change_events.write`

### escalation-policies

Escalation policies define which user should be alerted at which time.

- **`pagerduty-cli escalation-policies create-escalation-policy`** - Creates a new escalation policy. At least one escalation rule must be provided.

Escalation policies define which user should be alerted at which time.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `escalation_policies.write`
- **`pagerduty-cli escalation-policies delete-escalation-policy`** - Deletes an existing escalation policy and rules. The escalation policy must not be in use by any services.

Escalation policies define which user should be alerted at which time.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `escalation_policies.write`
- **`pagerduty-cli escalation-policies get-escalation-policy`** - Get information about an existing escalation policy and its rules.

Escalation policies define which user should be alerted at which time.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `escalation_policies.read`
- **`pagerduty-cli escalation-policies list`** - List all of the existing escalation policies.

Escalation policies define which user should be alerted at which time.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `escalation_policies.read`
- **`pagerduty-cli escalation-policies update-escalation-policy`** - Updates an existing escalation policy and rules.

Escalation policies define which user should be alerted at which time.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `escalation_policies.write`

### event-orchestrations

Event Orchestrations allow you to route events to an endpoint and create collections of Event Orchestrations, which define sets of actions to take based on event content.

- **`pagerduty-cli event-orchestrations create-cache-var-on-service-orch`** - Create a Cache Variable for a Service Event Orchestration.

Cache Variables allow you to store event data on an Event Orchestration, which can then be used in Event Orchestration rules as part of conditions or actions.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.write`
- **`pagerduty-cli event-orchestrations delete-cache-var-on-service-orch`** - Delete a Cache Variable for a Service Event Orchestration.

Cache Variables allow you to store event data on an Event Orchestration, which can then be used in Event Orchestration rules as part of conditions or actions.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.write`
- **`pagerduty-cli event-orchestrations delete-orchestration`** - Delete a Global Event Orchestration.

Once deleted, you will no longer be able to ingest events into PagerDuty using this Orchestration's Routing Key.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `event_orchestrations.write`
- **`pagerduty-cli event-orchestrations get-cache-var-on-service-orch`** - Get a Cache Variable for a Service Event Orchestration.

Cache Variables allow you to store event data on an Event Orchestration, which can then be used in Event Orchestration rules as part of conditions or actions.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.read`
- **`pagerduty-cli event-orchestrations get-orch-active-status`** - Get a Service Orchestration's active status.

A Service Orchestration allows you to set an active status based on whether an event will be evaluated against a service orchestration path (true) or service ruleset (false).

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.read`
- **`pagerduty-cli event-orchestrations get-orch-path-service`** - Get a Service Orchestration.

A Service Orchestration allows you to create a set of Event Rules. The Service Orchestration evaluates Events sent to this Service against each of its rules, beginning with the rules in the "start" set. When a matching rule is found, it can modify and enhance the event and can route the event to another set of rules within this Service Orchestration for further processing.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.read`
- **`pagerduty-cli event-orchestrations get-orchestration`** - Get a Global Event Orchestration.

Global Event Orchestrations allow you define a set of Global Rules and Router Rules, so that when you ingest events using the Orchestration's Routing Key your events will have actions applied via the Global Rules & then routed to the correct Service by the Router Rules, based on the event's content.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `event_orchestrations.read`
- **`pagerduty-cli event-orchestrations list`** - List all Global Event Orchestrations on an Account.

Global Event Orchestrations allow you define a set of Global Rules and Router Rules, so that when you ingest events using the Orchestration's Routing Key your events will have actions applied via the Global Rules & then routed to the correct Service by the Router Rules, based on the event's content.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `event_orchestrations.read`
- **`pagerduty-cli event-orchestrations list-cache-var-on-service-orch`** - List Cache Variables for a Service Event Orchestration.

Cache Variables allow you to store event data on an Event Orchestration, which can then be used in Event Orchestration rules as part of conditions or actions.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.read`
- **`pagerduty-cli event-orchestrations post-orchestration`** - Create a Global Event Orchestration.

Global Event Orchestrations allow you define a set of Global Rules and Router Rules, so that when you ingest events using the Orchestration's Routing Key your events will have actions applied via the Global Rules & then routed to the correct Service by the Router Rules, based on the event's content.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `event_orchestrations.write`
- **`pagerduty-cli event-orchestrations update-cache-var-on-service-orch`** - Update a Cache Variable for a Service Event Orchestration.

Cache Variables allow you to store event data on an Event Orchestration, which can then be used in Event Orchestration rules as part of conditions or actions.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.write`
- **`pagerduty-cli event-orchestrations update-orch-active-status`** - Update a Service Orchestration's active status.

A Service Orchestration allows you to set an active status based on whether an event will be evaluated against a service orchestration path (true) or service ruleset (false).

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.write`
- **`pagerduty-cli event-orchestrations update-orch-path-service`** - Update a Service Orchestration.

A Service Orchestration allows you to create a set of Event Rules. The Service Orchestration evaluates Events sent to this Service against each of its rules, beginning with the rules in the "start" set. When a matching rule is found, it can modify and enhance the event and can route the event to another set of rules within this Service Orchestration for further processing.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.write`
- **`pagerduty-cli event-orchestrations update-orchestration`** - Update a Global Event Orchestration.

Global Event Orchestrations allow you define a set of Global Rules and Router Rules, so that when you ingest events using the Orchestration's Routing Key your events will have actions applied via the Global Rules & then routed to the correct Service by the Router Rules, based on the event's content.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `event_orchestrations.write`

### extension-schemas

A PagerDuty extension vendor represents a specific type of outbound extension such as Generic Webhook, Slack, ServiceNow.

- **`pagerduty-cli extension-schemas get`** - Get details about one specific extension vendor.

A PagerDuty extension vendor represents a specific type of outbound extension such as Generic Webhook, Slack, ServiceNow.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `extension_schemas.read`
- **`pagerduty-cli extension-schemas list`** - List all extension schemas.

A PagerDuty extension vendor represents a specific type of outbound extension such as Generic Webhook, Slack, ServiceNow.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `extension_schemas.read`

### extensions

Extensions are representations of Extension Schema objects that are attached to Services.

- **`pagerduty-cli extensions create`** - Create a new Extension.

Extensions are representations of Extension Schema objects that are attached to Services.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `extensions.write`
- **`pagerduty-cli extensions delete`** - Delete an existing extension.

Once the extension is deleted, it will not be accessible from the web UI and new incidents won't be able to be created for this extension.

Extensions are representations of Extension Schema objects that are attached to Services.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `extensions.write`
- **`pagerduty-cli extensions get`** - Get details about an existing extension.

Extensions are representations of Extension Schema objects that are attached to Services.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `extensions.read`
- **`pagerduty-cli extensions list`** - List existing extensions.

Extensions are representations of Extension Schema objects that are attached to Services.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `extensions.read`
- **`pagerduty-cli extensions update`** - Update an existing extension.

Extensions are representations of Extension Schema objects that are attached to Services.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `extensions.write`

### incident-workflows

An Incident Workflow is a sequence of configurable Steps and associated Triggers that can execute automated Actions for a given Incident.

- **`pagerduty-cli incident-workflows associate-service-to-trigger`** - Associate a Service with an existing Incident Workflow Trigger

Scoped OAuth requires: `incident_workflows.write`
- **`pagerduty-cli incident-workflows create-trigger`** - Create new Incident Workflow Trigger

Scoped OAuth requires: `incident_workflows.write`
- **`pagerduty-cli incident-workflows delete`** - Delete an existing Incident Workflow

An Incident Workflow is a sequence of configurable Steps and associated Triggers that can execute automated Actions for a given Incident.

Scoped OAuth requires: `incident_workflows.write`
- **`pagerduty-cli incident-workflows delete-service-from-trigger`** - Remove a an existing Service from an Incident Workflow Trigger

Scoped OAuth requires: `incident_workflows.write`
- **`pagerduty-cli incident-workflows delete-trigger`** - Delete an existing Incident Workflow Trigger

Scoped OAuth requires: `incident_workflows.write`
- **`pagerduty-cli incident-workflows get`** - Get an existing Incident Workflow

An Incident Workflow is a sequence of configurable Steps and associated Triggers that can execute automated Actions for a given Incident.

Scoped OAuth requires: `incident_workflows.read`
- **`pagerduty-cli incident-workflows get-action`** - Get an Incident Workflow Action

Scoped OAuth requires: `incident_workflows.read`
- **`pagerduty-cli incident-workflows get-trigger`** - Retrieve an existing Incident Workflows Trigger

Scoped OAuth requires: `incident_workflows.read`
- **`pagerduty-cli incident-workflows list`** - List existing Incident Workflows.

This is the best method to use to list all Incident Workflows in your account. If your use case requires listing Incident Workflows associated with a particular Service, you can use the "List Triggers" method to find Incident Workflows configured to start for Incidents in a given Service.

An Incident Workflow is a sequence of configurable Steps and associated Triggers that can execute automated Actions for a given Incident.

Scoped OAuth requires: `incident_workflows.read`
- **`pagerduty-cli incident-workflows list-actions`** - List Incident Workflow Actions

Scoped OAuth requires: `incident_workflows.read`
- **`pagerduty-cli incident-workflows list-triggers`** - List existing Incident Workflow Triggers

Scoped OAuth requires: `incident_workflows.read`
- **`pagerduty-cli incident-workflows post`** - Create a new Incident Workflow

An Incident Workflow is a sequence of configurable Steps and associated Triggers that can execute automated Actions for a given Incident.

Scoped OAuth requires: `incident_workflows.write`
- **`pagerduty-cli incident-workflows put`** - Update an Incident Workflow

An Incident Workflow is a sequence of configurable Steps and associated Triggers that can execute automated Actions for a given Incident.

Scoped OAuth requires: `incident_workflows.write`
- **`pagerduty-cli incident-workflows update-trigger`** - Update an existing Incident Workflow Trigger

Scoped OAuth requires: `incident_workflows.write`

### incidents

An incident represents a problem or an issue that needs to be addressed and resolved. Incidents trigger on a service, which prompts notifications to go out to on-call responders per the service's escalation policy.

- **`pagerduty-cli incidents create`** - Create an incident synchronously without a corresponding event from a monitoring service.

An incident represents a problem or an issue that needs to be addressed and resolved.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `incidents.write`

This API operation has operation specific rate limits. See the [Rate Limits](https://developer.pagerduty.com/docs/72d3b724589e3-rest-api-rate-limits) page for more information.
- **`pagerduty-cli incidents create-custom-fields-field`** - <!-- theme: warning -->
> ### Deprecated
> This endpoint is deprecated and only works for fields on the Base Incident Type. \
> For more flexibility, we recommend using the Incident Types endpoint: \
> `/incidents/types/{type_id_or_name}/custom_fields`

Creates a new Custom Field on the Base Incident Type, along with the Field Options if provided. \
An account may have up to 10 Fields.

Scoped OAuth requires: `custom_fields.write`
- **`pagerduty-cli incidents create-custom-fields-field-option`** - <!-- theme: warning -->
> ### Deprecated
> This endpoint is deprecated and only works for fields on the Base Incident Type. \
> For more flexibility, we recommend using the Incident Types endpoint: \
> `/incidents/types/{type_id_or_name}/custom_fields/{field_id}/field_options`

Create a new Field Option for a Custom Field on the Base Incident Type. Field Options may only be created for Fields that have `field_options`. A Field may have no more than 10 enabled options.

Scoped OAuth requires: `custom_fields.write`
- **`pagerduty-cli incidents create-type`** - Create a new incident type.

Incident Types are a feature which will allow customers to categorize incidents, such as a security incident, a major incident, or a fraud incident.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `incident_types.write`
- **`pagerduty-cli incidents create-type-custom-field`** - Create a Custom Field for an Incident Type

Custom Fields (CF) are a feature which will allow customers to extend Incidents with their own custom data,
to provide additional context and support features such as customized filtering, search and analytics.
Custom Fields can be applied to different incident types.

Scoped OAuth requires: `custom_fields.write`
- **`pagerduty-cli incidents create-type-custom-field-field-options`** - Create a field option for a custom field.

Custom Fields (CF) are a feature which will allow customers to extend Incidents with their own custom data,
to provide additional context and support features such as customized filtering, search and analytics.
Custom Fields can be applied to different incident types.

Scoped OAuth requires: `custom_fields.write`
- **`pagerduty-cli incidents delete-custom-fields-field`** - <!-- theme: warning -->
> ### Deprecated
> This endpoint is deprecated and only works for fields on the Base Incident Type. \
> For more flexibility, we recommend using the Incident Types endpoint: \
> `/incidents/types/{type_id_or_name}/custom_fields/{field_id}`

Delete a Custom Field from the Base Incident Type.

Scoped OAuth requires: `custom_fields.write`
- **`pagerduty-cli incidents delete-custom-fields-field-option`** - <!-- theme: warning -->
> ### Deprecated
> This endpoint is deprecated and only works for fields on the Base Incident Type. \
> For more flexibility, we recommend using the Incident Types endpoint: \
> `/incidents/types/{type_id_or_name}/custom_fields/{field_id}/field_options/{field_option_id}`

Delete a Field Option for a Custom Field on the Base Incident Type.

Scoped OAuth requires: `custom_fields.write`
- **`pagerduty-cli incidents delete-type-custom-field`** - Delete a custom field for an incident type.

Custom Fields (CF) are a feature which will allow customers to extend Incidents with their own custom data,
to provide additional context and support features such as customized filtering, search and analytics.
Custom Fields can be applied to different incident types.

Scoped OAuth requires: `custom_fields.write`
- **`pagerduty-cli incidents delete-type-custom-field-field-option`** - Delete a field option for a custom field.

Custom Fields (CF) are a feature which will allow customers to extend Incidents with their own custom data,
to provide additional context and support features such as customized filtering, search and analytics.
Custom Fields can be applied to different incident types.

Scoped OAuth requires: `custom_fields.write`
- **`pagerduty-cli incidents get`** - Show detailed information about an incident. Accepts either an incident id, or an incident number.

An incident represents a problem or an issue that needs to be addressed and resolved.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `incidents.read`
- **`pagerduty-cli incidents get-custom-fields-field`** - <!-- theme: warning -->
> ### Deprecated
> This endpoint is deprecated and only works for fields on the Base Incident Type. \
> For more flexibility, we recommend using the Incident Types endpoint: \
> `/incidents/types/{type_id_or_name}/custom_fields/{field_id}`

Show detailed information about a Custom Field on the Base Incident Type.

Scoped OAuth requires: `custom_fields.read`
- **`pagerduty-cli incidents get-type`** - Get detailed information about a single incident type. Accepts either an incident type id, or an incident type name.

Incident Types are a feature which will allow customers to categorize incidents, such as a security incident, a major incident, or a fraud incident.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `incident_types.read`
- **`pagerduty-cli incidents get-type-custom-field`** - Get a custom field for an incident type.

Custom Fields (CF) are a feature which will allow customers to extend Incidents with their own custom data,
to provide additional context and support features such as customized filtering, search and analytics.
Custom Fields can be applied to different incident types.

Scoped OAuth requires: `custom_fields.read`
- **`pagerduty-cli incidents get-type-custom-field-field-options`** - Get a field option on a custom field

Custom Fields (CF) are a feature which will allow customers to extend Incidents with their own custom data,
to provide additional context and support features such as customized filtering, search and analytics.
Custom Fields can be applied to different incident types.

Scoped OAuth requires: `custom_fields.read`
- **`pagerduty-cli incidents list`** - List existing incidents.

An incident represents a problem or an issue that needs to be addressed and resolved.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `incidents.read`
- **`pagerduty-cli incidents list-custom-fields-field-options`** - <!-- theme: warning -->
> ### Deprecated
> This endpoint is deprecated and only works for fields on the Base Incident Type. \
> For more flexibility, we recommend using the Incident Types endpoint: \
> `/incidents/types/{type_id_or_name}/custom_fields/{field_id}/field_options`

List all enabled Field Options for a Custom Field on the Base Incident Type.

Scoped OAuth requires: `custom_fields.read`
- **`pagerduty-cli incidents list-custom-fields-fields`** - <!-- theme: warning -->
> ### Deprecated
> This endpoint is deprecated and only works for fields on the Base Incident Type. \
> For more flexibility, we recommend using the Incident Types endpoint: \
> `/incidents/types/{type_id_or_name}/custom_fields`

List Custom Fields on the Base Incident Type.

Scoped OAuth requires: `custom_fields.read`
- **`pagerduty-cli incidents list-type-custom-field`** - List field options for a custom field.

Custom Fields (CF) are a feature which will allow customers to extend Incidents with their own custom data,
to provide additional context and support features such as customized filtering, search and analytics.
Custom Fields can be applied to different incident types.

Scoped OAuth requires: `custom_fields.read`
- **`pagerduty-cli incidents list-type-custom-fields`** - List the custom fields for an incident type.

Custom Fields (CF) are a feature which will allow customers to extend Incidents with their own custom data,
to provide additional context and support features such as customized filtering, search and analytics.
Custom Fields can be applied to different incident types.

Scoped OAuth requires: `custom_fields.read`
- **`pagerduty-cli incidents list-types`** - List the available incident types

Incident Types are a feature which will allow customers to categorize incidents, such as a security incident, a major incident, or a fraud incident.
These can be filtered by enabled or disabled types.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `incident_types.read`
- **`pagerduty-cli incidents update`** - Acknowledge, resolve, escalate or reassign one or more incidents.

An incident represents a problem or an issue that needs to be addressed and resolved.

A maximum of 250 incidents may be updated at a time. If more than this number of incidents are given, the API will respond with status 413 (Request Entity Too Large).

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `incidents.write`

This API operation has operation specific rate limits. See the [Rate Limits](https://developer.pagerduty.com/docs/72d3b724589e3-rest-api-rate-limits) page for more information.
- **`pagerduty-cli incidents update-custom-fields-field`** - <!-- theme: warning -->
> ### Deprecated
> This endpoint is deprecated and only works for fields on the Base Incident Type. \
> For more flexibility, we recommend using the Incident Types endpoint: \
> `/incidents/types/{type_id_or_name}/custom_fields/{field_id}`

Update a Custom Field on the Base Incident Type.

Scoped OAuth requires: `custom_fields.write`
- **`pagerduty-cli incidents update-custom-fields-field-option`** - <!-- theme: warning -->
> ### Deprecated
> This endpoint is deprecated and only works for fields on the Base Incident Type. \
> For more flexibility, we recommend using the Incident Types endpoint: \
> `/incidents/types/{type_id_or_name}/custom_fields/{field_id}/field_options/{field_option_id}`

Update a Field Option for a Custom Field on the Base Incident Type.

Scoped OAuth requires: `custom_fields.write`
- **`pagerduty-cli incidents update-id`** - Acknowledge, resolve, escalate or reassign an incident.

An incident represents a problem or an issue that needs to be addressed and resolved.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `incidents.write`
- **`pagerduty-cli incidents update-type`** - Update an Incident Type.

Incident Types are a feature which will allow customers to categorize incidents, such as a security incident, a major incident, or a fraud incident.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `incident_types.write`
- **`pagerduty-cli incidents update-type-custom-field`** - Update a custom field for an incident type. Field Options can also be updated within the same call.

Custom Fields (CF) are a feature which will allow customers to extend Incidents with their own custom data,
to provide additional context and support features such as customized filtering, search and analytics.
Custom Fields can be applied to different incident types.

Scoped OAuth requires: `custom_fields.write`
- **`pagerduty-cli incidents update-type-custom-field-field-option`** - Update a field option for a custom field.

Custom Fields (CF) are a feature which will allow customers to extend Incidents with their own custom data,
to provide additional context and support features such as customized filtering, search and analytics.
Custom Fields can be applied to different incident types.

Scoped OAuth requires: `custom_fields.write`

### ip-allow-lists

Manage account-level IP Allow Lists that restrict access to your PagerDuty subdomain to a configured set of IPv4 CIDR ranges. Enforcement currently applies to web and mobile application traffic.


<!-- theme: warning -->

> ### Early Access
> This API is in Early Access and may change at any time. You must pass the `X-EARLY-ACCESS: ip-allow-lists` header on every request, and your account must be enrolled in the IP Allow Lists Early Access program. Contact your PagerDuty account team to request access.

- **`pagerduty-cli ip-allow-lists create`** - <!-- theme: warning -->

> ### Early Access
> This API is in Early Access and may change at any time. You must pass the `X-EARLY-ACCESS: ip-allow-lists` header on every request, and your account must be enrolled in the IP Allow Lists Early Access program. Contact your PagerDuty account team to request access.

Create the account's IP allow list.

Only Account Owners, Global Admins, and Account API Keys can call this endpoint.

Scoped OAuth requires: `ip_allow_lists.write`
- **`pagerduty-cli ip-allow-lists delete`** - <!-- theme: warning -->

> ### Early Access
> This API is in Early Access and may change at any time. You must pass the `X-EARLY-ACCESS: ip-allow-lists` header on every request, and your account must be enrolled in the IP Allow Lists Early Access program. Contact your PagerDuty account team to request access.

Delete the IP allow list with the given `id`. Subsequent `GET` and `PUT` requests for the same `id` will return `404`. The list is no longer enforced once deleted.

Only Account Owners, Global Admins, and Account API Keys can call this endpoint.

Scoped OAuth requires: `ip_allow_lists.write`
- **`pagerduty-cli ip-allow-lists get`** - <!-- theme: warning -->

> ### Early Access
> This API is in Early Access and may change at any time. You must pass the `X-EARLY-ACCESS: ip-allow-lists` header on every request, and your account must be enrolled in the IP Allow Lists Early Access program. Contact your PagerDuty account team to request access.

Return the IP allow list with the given `id`.

Only Account Owners, Global Admins, and Account API Keys can call this endpoint.

Scoped OAuth requires: `ip_allow_lists.read`
- **`pagerduty-cli ip-allow-lists list`** - <!-- theme: warning -->

> ### Early Access
> This API is in Early Access and may change at any time. You must pass the `X-EARLY-ACCESS: ip-allow-lists` header on every request, and your account must be enrolled in the IP Allow Lists Early Access program. Contact your PagerDuty account team to request access.

Return all IP allow lists for the account.

Only Account Owners, Global Admins, and Account API Keys can call this endpoint.

Scoped OAuth requires: `ip_allow_lists.read`
- **`pagerduty-cli ip-allow-lists update`** - <!-- theme: warning -->

> ### Early Access
> This API is in Early Access and may change at any time. You must pass the `X-EARLY-ACCESS: ip-allow-lists` header on every request, and your account must be enrolled in the IP Allow Lists Early Access program. Contact your PagerDuty account team to request access.

Update the IP allow list with the given `id`. The request body fully replaces the writable fields.

Only Account Owners, Global Admins, and Account API Keys can call this endpoint.

Scoped OAuth requires: `ip_allow_lists.write`

### license-allocations

Manage license allocations

- **`pagerduty-cli license-allocations`** - List the Licenses allocated to Users within your Account

Scoped OAuth requires: `licenses.read`

### licenses

Licenses are allocated to Users to allow for per-User access to PagerDuty functionality within an Account.

- **`pagerduty-cli licenses`** - List the Licenses associated with your Account

Scoped OAuth requires: `licenses.read`

### log-entries

A log of all the events that happen to an Incident, and these are exposed as Log Entries.

- **`pagerduty-cli log-entries get-log-entry`** - Get details for a specific incident log entry. This method provides additional information you can use to get at raw event data.

A log of all the events that happen to an Incident, and these are exposed as Log Entries.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `incidents.read`
- **`pagerduty-cli log-entries list`** - List all of the incident log entries across the entire account.

A log of all the events that happen to an Incident, and these are exposed as Log Entries.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `incidents.read`

### maintenance-windows

A Maintenance Window is used to temporarily disable one or more Services for a set period of time.

- **`pagerduty-cli maintenance-windows create`** - Create a new maintenance window for the specified services. No new incidents will be created for a service that is in maintenance.

A Maintenance Window is used to temporarily disable one or more Services for a set period of time.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.write`
- **`pagerduty-cli maintenance-windows delete`** - Delete an existing maintenance window if it's in the future, or end it if it's currently on-going. If the maintenance window has already ended it cannot be deleted.

A Maintenance Window is used to temporarily disable one or more Services for a set period of time.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.write`
- **`pagerduty-cli maintenance-windows get`** - Get an existing maintenance window.

A Maintenance Window is used to temporarily disable one or more Services for a set period of time.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.read`
- **`pagerduty-cli maintenance-windows list`** - List existing maintenance windows, optionally filtered by service and/or team, or whether they are from the past, present or future.

A Maintenance Window is used to temporarily disable one or more Services for a set period of time.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.read`
- **`pagerduty-cli maintenance-windows update`** - Update an existing maintenance window.

A Maintenance Window is used to temporarily disable one or more Services for a set period of time.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.write`

### notifications

A Notification is created when an Incident is triggered or escalated.

- **`pagerduty-cli notifications`** - List notifications for a given time range, optionally filtered by type (sms_notification, email_notification, phone_notification, or push_notification).

A Notification is created when an Incident is triggered or escalated.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `users:notifications.read`

### oauth-delegations

An OAuth Delegation represents a delegation of a User's permissions to an OAuth Client, allowing the client to impersonate the user when making API requests.

- **`pagerduty-cli oauth-delegations delete`** - Delete all OAuth delegations as per provided query parameters.

An OAuth delegation represents an instance of a user or account's authorization to an app (via OAuth) to access their PagerDuty account.
Common apps include the PagerDuty mobile app, Slack, Microsoft Teams, and third-party apps. It also represents a user session in the PagerDuty web app.

Deleting an OAuth delegation will revoke that instance of an app's access to that user or account.
To grant access again, reauthorization/reauthentication will be required.

This endpoint supports deleting mobile app OAuth delegations for a given user, which is equivalent to signing users out of the mobile app. It also supports deleting delegations of type web, which is equivalent to signing users out of the web app.

This is a synchronous API.

Scoped OAuth requires: `oauth_delegations.write`
- **`pagerduty-cli oauth-delegations get-revocation-requests-status`** - <!-- theme: warning -->
> ### Deprecated
> This endpoint is deprecated as OAuth token revocation is now synchronous. Please use the [DELETE /oauth_delegations endpoint](https://developer.pagerduty.com/api-reference/ad1161db75db1-delete-all-o-auth-delegations) instead.

Get the status of all OAuth delegations revocation requests for this account, specifically how many requests are still pending. As all requests are now synchronous, no pending requests will be found.

This endpoint is limited to account owners and admins.

Scoped OAuth requires: `oauth_delegations.read`

### oncalls

Manage oncalls

- **`pagerduty-cli oncalls`** - List the on-call entries during a given time range.

An on-call represents a contiguous unit of time for which a User will be on call for a given Escalation Policy and Escalation Rules.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `oncalls.read`

This API operation has operation specific rate limits. See the [Rate Limits](https://developer.pagerduty.com/docs/72d3b724589e3-rest-api-rate-limits) page for more information.

### pagerduty-analytics

Manage pagerduty analytics

- **`pagerduty-cli pagerduty-analytics get-incident-responses-by-id`** - Provides enriched responder data for a single incident.

Example metrics include Time to Respond, Responder Type, and Response Status. See metric definitions below.

<!-- theme: info -->
> **Note:** Analytics data is updated once per day. It takes up to 24 hours before new incident responses appear in the Analytics API.
Scoped OAuth requires: `analytics.read`
- **`pagerduty-cli pagerduty-analytics get-incidents`** - Provides enriched incident data and metrics for multiple incidents.

Example metrics include Seconds to Resolve, Seconds to Engage, Snoozed Seconds, and Sleep Hour Interruptions. Metric definitions can be found in our [Knowledge Base](https://support.pagerduty.com/docs/insights#incidents-list).

<!-- theme: info -->
> A `team_ids` or `service_ids` filter is required for [user-level API keys](https://support.pagerduty.com/docs/using-the-api#section-generating-a-personal-rest-api-key) or keys generated through an OAuth flow. Account-level API keys do not have this requirement.
<!-- theme: info -->
> **Note:** Analytics data is updated [periodically](https://support.pagerduty.com/main/docs/insights#:~:text=Data%20Update%20Schedule). It takes up to 24 hours before new incidents appear in the Analytics API.

Scoped OAuth requires: `analytics.write`
- **`pagerduty-cli pagerduty-analytics get-incidents-by-id`** - Provides enriched incident data and metrics for a single incident.

Example metrics include Seconds to Resolve, Seconds to Engage, Snoozed Seconds, and Sleep Hour Interruptions. Metric definitions can be found in our [Knowledge Base](https://support.pagerduty.com/docs/insights#incidents-list).

<!-- theme: info -->
> **Note:** Analytics data is updated [periodically](https://support.pagerduty.com/main/docs/insights#:~:text=Data%20Update%20Schedule). It takes up to 24 hours before new incidents appear in the Analytics API.

Scoped OAuth requires: `analytics.read`
- **`pagerduty-cli pagerduty-analytics get-metrics-incidents-all`** - Provides aggregated enriched metrics for incidents.

The provided metrics are aggregated by day, week, month using the aggregate_unit parameter, or for the entire period if no aggregate_unit is provided.

<!-- theme: info -->
> A `team_ids` or `service_ids` filter is required for [user-level API keys](https://support.pagerduty.com/docs/using-the-api#section-generating-a-personal-rest-api-key) or keys generated through an OAuth flow. Account-level API keys do not have this requirement.
<!-- theme: info -->
> **Note:** Analytics data is updated [periodically](https://support.pagerduty.com/main/docs/insights#:~:text=Data%20Update%20Schedule). It takes up to 24 hours before new incidents appear in the Analytics API.

Scoped OAuth requires: `analytics.write`
- **`pagerduty-cli pagerduty-analytics get-metrics-incidents-escalation-policy`** - Provides aggregated metrics for incidents aggregated into units of time by escalation policy.

Example metrics include Seconds to Resolve, Seconds to Engage, Snoozed Seconds, and Sleep Hour Interruptions. Metric definitions can be found in our [Knowledge Base](https://support.pagerduty.com/docs/insights#escalation-policy-list).

<!-- theme: info -->
> **Note:** Analytics data is updated [periodically](https://support.pagerduty.com/main/docs/insights#:~:text=Data%20Update%20Schedule). It takes up to 24 hours before new incidents appear in the Analytics API.

Scoped OAuth requires: `analytics.write`
- **`pagerduty-cli pagerduty-analytics get-metrics-incidents-escalation-policy-all`** - Provides aggregated metrics across all escalation policies.

Example metrics include Seconds to Resolve, Seconds to Engage, Snoozed Seconds, and Sleep Hour Interruptions. Metric definitions can be found in our [Knowledge Base](https://support.pagerduty.com/docs/insights#escalation-policy-list).

<!-- theme: info -->
> **Note:** Analytics data is updated [periodically](https://support.pagerduty.com/main/docs/insights#:~:text=Data%20Update%20Schedule). It takes up to 24 hours before new incidents appear in the Analytics API.

Scoped OAuth requires: `analytics.write`
- **`pagerduty-cli pagerduty-analytics get-metrics-incidents-service`** - Provides aggregated metrics for incidents aggregated into units of time by service.

Example metrics include Seconds to Resolve, Seconds to Engage, Snoozed Seconds, and Sleep Hour Interruptions. Metric definitions can be found in our [Knowledge Base](https://support.pagerduty.com/docs/insights#services-list).
Data can be aggregated by day, week or month in addition to by service, or provided just as a collection of aggregates for each service in the dataset for the entire period.  If a unit is provided, each row in the returned dataset will include a 'range_start' timestamp.

<!-- theme: info -->
> **Note:** Analytics data is updated [periodically](https://support.pagerduty.com/main/docs/insights#:~:text=Data%20Update%20Schedule). It takes up to 24 hours before new incidents appear in the Analytics API.

Scoped OAuth requires: `analytics.write`
- **`pagerduty-cli pagerduty-analytics get-metrics-incidents-service-all`** - Provides aggregated metrics across all services.

Example metrics include Seconds to Resolve, Seconds to Engage, Snoozed Seconds, and Sleep Hour Interruptions. Metric definitions can be found in our [Knowledge Base](https://support.pagerduty.com/docs/insights#services-list).

<!-- theme: info -->
> A `team_ids` or `service_ids` filter is required for [user-level API keys](https://support.pagerduty.com/docs/using-the-api#section-generating-a-personal-rest-api-key) or keys generated through an OAuth flow. Account-level API keys do not have this requirement.
<!-- theme: info -->
> **Note:** Analytics data is updated [periodically](https://support.pagerduty.com/main/docs/insights#:~:text=Data%20Update%20Schedule). It takes up to 24 hours before new incidents appear in the Analytics API.

Scoped OAuth requires: `analytics.write`
- **`pagerduty-cli pagerduty-analytics get-metrics-incidents-team`** - Provides aggregated metrics for incidents aggregated into units of time by team.

Example metrics include Seconds to Resolve, Seconds to Engage, Snoozed Seconds, and Sleep Hour Interruptions. Metric definitions can be found in our [Knowledge Base](https://support.pagerduty.com/docs/insights#teams-list).
Data can be aggregated by day, week or month in addition to by team, or provided just as a collection of aggregates for each team in the dataset for the entire period.  If a unit is provided, each row in the returned dataset will include a 'range_start' timestamp.

<!-- theme: info -->
> A `team_ids` or `service_ids` filter is required for [user-level API keys](https://support.pagerduty.com/docs/using-the-api#section-generating-a-personal-rest-api-key) or keys generated through an OAuth flow. Account-level API keys do not have this requirement.
<!-- theme: info -->
> **Note:** Analytics data is updated [periodically](https://support.pagerduty.com/main/docs/insights#:~:text=Data%20Update%20Schedule). It takes up to 24 hours before new incidents appear in the Analytics API.

Scoped OAuth requires: `analytics.write`
- **`pagerduty-cli pagerduty-analytics get-metrics-incidents-team-all`** - Provides aggregated metrics across all teams.

Example metrics include Seconds to Resolve, Seconds to Engage, Snoozed Seconds, and Sleep Hour Interruptions. Metric definitions can be found in our [Knowledge Base](https://support.pagerduty.com/docs/insights#teams-list).

<!-- theme: info -->
> A `team_ids` or `service_ids` filter is required for [user-level API keys](https://support.pagerduty.com/docs/using-the-api#section-generating-a-personal-rest-api-key) or keys generated through an OAuth flow. Account-level API keys do not have this requirement.
<!-- theme: info -->
> **Note:** Analytics data is updated [periodically](https://support.pagerduty.com/main/docs/insights#:~:text=Data%20Update%20Schedule). It takes up to 24 hours before new incidents appear in the Analytics API.

Scoped OAuth requires: `analytics.write`
- **`pagerduty-cli pagerduty-analytics get-metrics-pd-advance-usage-features`** - Provides aggregated metrics for the usage of PD Advance.
<!-- theme: info -->
> **Note:** Analytics data is updated [periodically](https://support.pagerduty.com/main/docs/insights#:~:text=Data%20Update%20Schedule). It takes up to 24 hours before new incidents appear in the Analytics API.

Scoped OAuth requires: `analytics.write`
- **`pagerduty-cli pagerduty-analytics get-metrics-responders-all`** - Provides aggregated incident metrics for all selected responders.

Example metrics include Seconds to Resolve, Seconds to Engage, Snoozed Seconds, and Sleep Hour Interruptions. Metric definitions can be found in our [Knowledge Base](https://support.pagerduty.com/docs/insights#responders-list).

<!-- theme: info -->
> **Note:** Analytics data is updated [periodically](https://support.pagerduty.com/main/docs/insights#:~:text=Data%20Update%20Schedule). It takes up to 24 hours before new incidents appear in the Analytics API.

Scoped OAuth requires: `analytics.write`
- **`pagerduty-cli pagerduty-analytics get-metrics-responders-team`** - Provides incident metrics aggregated by responder.

Example metrics include Seconds to Resolve, Seconds to Engage, Snoozed Seconds, and Sleep Hour Interruptions. Metric definitions can be found in our [Knowledge Base](https://support.pagerduty.com/docs/insights#responders-list).

<!-- theme: info -->
> **Note:** Analytics data is updated [periodically](https://support.pagerduty.com/main/docs/insights#:~:text=Data%20Update%20Schedule). It takes up to 24 hours before new incidents appear in the Analytics API.

Scoped OAuth requires: `analytics.write`
- **`pagerduty-cli pagerduty-analytics get-metrics-users-all`** - Provides aggregated metrics across all users within their account. This endpoint provides summary statistics about user activity and performance.

<!-- theme: info -->
> **Note:** Analytics data is updated [periodically](https://support.pagerduty.com/main/docs/insights#:~:text=Data%20Update%20Schedule). It takes up to 24 hours before new incidents appear in the Analytics API.

Scoped OAuth requires: `analytics.write`
- **`pagerduty-cli pagerduty-analytics get-responder-incidents`** - Provides enriched incident data and metrics for a specific responder.

Example metrics include Mean Seconds to Resolve, Mean Seconds to Engage, Snoozed Seconds, and Sleep Hour Interruptions. Metric definitions can be found in our [Knowledge Base](https://support.pagerduty.com/docs/insights#incidents-list).

<!-- theme: info -->
> **Note:** Analytics data is updated [periodically](https://support.pagerduty.com/main/docs/insights#:~:text=Data%20Update%20Schedule). It takes up to 24 hours before new incidents appear in the Analytics API.

Scoped OAuth requires: `analytics.write`
- **`pagerduty-cli pagerduty-analytics get-users`** - Allows users to retrieve a raw list of user analytics data within their account. This endpoint provides detailed data about user activity and account configuration.

<!-- theme: info -->
> **Note:** Analytics data is updated [periodically](https://support.pagerduty.com/main/docs/insights#:~:text=Data%20Update%20Schedule). It takes up to 24 hours before new user data appears in the Analytics API.

Scoped OAuth requires: `analytics.write`

### paused-incident-reports

Provides paused Incident reporting data on services and accounts that have paused Alerts.

- **`pagerduty-cli paused-incident-reports get-alerts`** - Returns the 5 most recent alerts that were triggered after being paused and the 5 most recent alerts that were resolved after being paused for a given reporting period (maximum 6 months lookback period).  Note: This feature is currently available as part of the Event Intelligence package or Digital Operations plan only.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `incidents.read`
- **`pagerduty-cli paused-incident-reports get-counts`** - Returns reporting counts for paused Incident usage for a given reporting period (maximum 6 months lookback period).  Note: This feature is currently available as part of the Event Intelligence package or Digital Operations plan only.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `incidents.read`

### priorities

A priority is a label representing the importance and impact of an incident. This feature is only available on Standard and Enterprise plans.

- **`pagerduty-cli priorities`** - List existing priorities, in order (most to least severe).

A priority is a label representing the importance and impact of an incident.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `priorities.read`

### rulesets

Rulesets allow you to route events to an endpoint and create collections of Event Rules, which define sets of actions to take based on event content.

- **`pagerduty-cli rulesets create`** - Create a new Ruleset.
<!-- theme: warning -->
> ### End-of-life
> Rulesets and Event Rules will end-of-life soon. We highly recommend that you [migrate to Event Orchestration](https://support.pagerduty.com/docs/migrate-to-event-orchestration) as soon as possible so you can take advantage of the new functionality, such as improved UI, rule creation, APIs and Terraform support, advanced conditions, and rule nesting.

Rulesets allow you to route events to an endpoint and create collections of Event Rules, which define sets of actions to take based on event content.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `event_rules.write`
- **`pagerduty-cli rulesets delete`** - Delete a Ruleset.
<!-- theme: warning -->
> ### End-of-life
> Rulesets and Event Rules will end-of-life soon. We highly recommend that you [migrate to Event Orchestration](https://support.pagerduty.com/docs/migrate-to-event-orchestration) as soon as possible so you can take advantage of the new functionality, such as improved UI, rule creation, APIs and Terraform support, advanced conditions, and rule nesting.

Rulesets allow you to route events to an endpoint and create collections of Event Rules, which define sets of actions to take based on event content.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `event_rules.write`
- **`pagerduty-cli rulesets get`** - Get a Ruleset.
<!-- theme: warning -->
> ### End-of-life
> Rulesets and Event Rules will end-of-life soon. We highly recommend that you [migrate to Event Orchestration](https://support.pagerduty.com/docs/migrate-to-event-orchestration) as soon as possible so you can take advantage of the new functionality, such as improved UI, rule creation, APIs and Terraform support, advanced conditions, and rule nesting.

Rulesets allow you to route events to an endpoint and create collections of Event Rules, which define sets of actions to take based on event content.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `event_rules.read`
- **`pagerduty-cli rulesets list`** - List all Rulesets
<!-- theme: warning -->
> ### End-of-life
> Rulesets and Event Rules will end-of-life soon. We highly recommend that you [migrate to Event Orchestration](https://support.pagerduty.com/docs/migrate-to-event-orchestration) as soon as possible so you can take advantage of the new functionality, such as improved UI, rule creation, APIs and Terraform support, advanced conditions, and rule nesting.

Rulesets allow you to route events to an endpoint and create collections of Event Rules, which define sets of actions to take based on event content.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `event_rules.read`
- **`pagerduty-cli rulesets update`** - Update a Ruleset.
<!-- theme: warning -->
> ### End-of-life
> Rulesets and Event Rules will end-of-life soon. We highly recommend that you [migrate to Event Orchestration](https://support.pagerduty.com/docs/migrate-to-event-orchestration) as soon as possible so you can take advantage of the new functionality, such as improved UI, rule creation, APIs and Terraform support, advanced conditions, and rule nesting.

Rulesets allow you to route events to an endpoint and create collections of Event Rules, which define sets of actions to take based on event content.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `event_rules.write`

### schedules

A Schedule determines the time periods that users are On-Call.

- **`pagerduty-cli schedules create`** - Create a new on-call schedule.

A Schedule determines the time periods that users are On-Call.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `schedules.write`
- **`pagerduty-cli schedules create-preview`** - Preview what an on-call schedule would look like without saving it.

A Schedule determines the time periods that users are On-Call.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `schedules.write`
- **`pagerduty-cli schedules create-v3`** - <!-- theme: info -->

> **Important note:** Shift-based schedules use the V3 API and are not compatible with V2 automations. **To create automations for Shift-Based Schedules, you need to:**
>
> 1. **Update your automations** to use the V3 API for all new shift-based schedules
> 2. **Keep the V2 endpoint** for your existing schedules
>
> An upgrade tool for existing schedules is coming soon; your legacy schedules will keep working in the meantime. [Learn more](https://support.pagerduty.com/main/docs/shift-based-schedules-api-upgrade-examples).

Create a new on-call schedule with basic metadata. Rotations and events
must be added via separate API calls after creation.

**Rejected fields:** `rotations` and `escalation_policies` are not
accepted in the request body and will result in a 400 error.
- **`pagerduty-cli schedules delete`** - Delete an on-call schedule.

A Schedule determines the time periods that users are On-Call.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `schedules.write`
- **`pagerduty-cli schedules delete-v3`** - <!-- theme: info -->

> **Important note:** Shift-based schedules use the V3 API and are not compatible with V2 automations. **To create automations for Shift-Based Schedules, you need to:**
>
> 1. **Update your automations** to use the V3 API for all new shift-based schedules
> 2. **Keep the V2 endpoint** for your existing schedules
>
> An upgrade tool for existing schedules is coming soon; your legacy schedules will keep working in the meantime. [Learn more](https://support.pagerduty.com/main/docs/shift-based-schedules-api-upgrade-examples).

Delete a schedule and all associated rotations and events.

If the schedule is referenced by an active escalation policy, the
deletion will be rejected.
- **`pagerduty-cli schedules get`** - Show detailed information about a schedule, including entries for each layer.
Scoped OAuth requires: `schedules.read`
- **`pagerduty-cli schedules get-v3`** - <!-- theme: info -->

> **Important note:** Shift-based schedules use the V3 API and are not compatible with V2 automations. **To create automations for Shift-Based Schedules, you need to:**
>
> 1. **Update your automations** to use the V3 API for all new shift-based schedules
> 2. **Keep the V2 endpoint** for your existing schedules
>
> An upgrade tool for existing schedules is coming soon; your legacy schedules will keep working in the meantime. [Learn more](https://support.pagerduty.com/main/docs/shift-based-schedules-api-upgrade-examples).

Retrieve a schedule by ID including rotations and events. Optionally
include the computed final schedule for a time range.

Use `include[]=final_schedule` to get computed on-call assignments.
Use `since` and `until` to specify the time range.
- **`pagerduty-cli schedules list`** - List the on-call schedules.

A Schedule determines the time periods that users are On-Call.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `schedules.read`
- **`pagerduty-cli schedules list-v3`** - <!-- theme: info -->

> **Important note:** Shift-based schedules use the V3 API and are not compatible with V2 automations. **To create automations for Shift-Based Schedules, you need to:**
>
> 1. **Update your automations** to use the V3 API for all new shift-based schedules
> 2. **Keep the V2 endpoint** for your existing schedules
>
> An upgrade tool for existing schedules is coming soon; your legacy schedules will keep working in the meantime. [Learn more](https://support.pagerduty.com/main/docs/shift-based-schedules-api-upgrade-examples).

Retrieve a paginated list of schedule references. Returns lightweight
objects without embedded rotations or events.

Each result is filtered by the caller's read permission; schedules the
caller cannot read are silently excluded.
- **`pagerduty-cli schedules update`** - Update an existing on-call schedule.

A Schedule determines the time periods that users are On-Call.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `schedules.write`
- **`pagerduty-cli schedules update-v3`** - <!-- theme: info -->

> **Important note:** Shift-based schedules use the V3 API and are not compatible with V2 automations. **To create automations for Shift-Based Schedules, you need to:**
>
> 1. **Update your automations** to use the V3 API for all new shift-based schedules
> 2. **Keep the V2 endpoint** for your existing schedules
>
> An upgrade tool for existing schedules is coming soon; your legacy schedules will keep working in the meantime. [Learn more](https://support.pagerduty.com/main/docs/shift-based-schedules-api-upgrade-examples).

Update schedule metadata (name, description, time zone). All fields are
optional  -  only provided fields are updated.

To modify rotations or events, use their respective endpoints.

**Rejected fields:** `rotations` and `escalation_policies` are not
accepted and will result in a 400 error.

### service-dependencies

Services are categorized into technical and business services. Dependencies can be created via any combination of these services.

- **`pagerduty-cli service-dependencies create-service-dependency`** - Create new dependencies between two services.

Business services model capabilities that span multiple technical services and that may be owned by several different teams.

A service can have a maximum of 2,000 dependencies with a depth limit of 100. If the limit is reached, the API will respond with an error.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.write`
- **`pagerduty-cli service-dependencies delete-service-dependency`** - Disassociate dependencies between two services.

Business services model capabilities that span multiple technical services and that may be owned by several different teams.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.write`
- **`pagerduty-cli service-dependencies get-business-service`** - Get all immediate dependencies of any Business Service.

Business Services model capabilities that span multiple technical services and that may be owned by several different teams.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.read`
- **`pagerduty-cli service-dependencies get-technical-service`** - Get all immediate dependencies of any technical service.
Technical services are also known as `services`.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.read`

### services

A Service may represent an application, component, or team you wish to open incidents against.

- **`pagerduty-cli services create`** - Create a new service.

If `status` is included in the request, it must have a value of `active` when creating a new service. If a different status is required, make a second request to update the service.

A service may represent an application, component, or team you wish to open incidents against.

There is a limit of 25,000 services per account. If the limit is reached, the API will respond with an error. There is also a limit of 100,000 open Incidents per Service. If the limit is reached and `auto_resolve_timeout` is disabled (set to 0 or null), the `auto_resolve_timeout` property will automatically be set to  84600 (1 day).

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.write`
- **`pagerduty-cli services create-custom-field`** - Creates a new Custom Field for Services, along with the Field Options if provided.

Scoped OAuth requires: `custom_fields.write`
- **`pagerduty-cli services create-custom-field-option`** - Create a new option for the given field.

Scoped OAuth requires: `custom_fields.write`
- **`pagerduty-cli services delete`** - Delete an existing service.

Once the service is deleted, it will not be accessible from the web UI and new incidents won't be able to be created for this service.

A service may represent an application, component, or team you wish to open incidents against.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.write`
- **`pagerduty-cli services delete-custom-field`** - Delete a Custom Field from Services.

Scoped OAuth requires: `custom_fields.write`
- **`pagerduty-cli services delete-custom-field-option`** - Delete a field option.

Scoped OAuth requires: `custom_fields.write`
- **`pagerduty-cli services get`** - Get details about an existing service.

A service may represent an application, component, or team you wish to open incidents against.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.read`
- **`pagerduty-cli services get-custom-field`** - Show detailed information about a Custom Field for Services.

Scoped OAuth requires: `custom_fields.read`
- **`pagerduty-cli services get-custom-field-option`** - Get a field option for a given field.

Scoped OAuth requires: `custom_fields.read`
- **`pagerduty-cli services list`** - List existing Services.

A service may represent an application, component, or team you wish to open incidents against.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.read`
- **`pagerduty-cli services list-custom-field-options`** - List all options for a given field.

Scoped OAuth requires: `custom_fields.read`
- **`pagerduty-cli services list-custom-fields`** - List Custom Fields available for Services.

Scoped OAuth requires: `custom_fields.read`
- **`pagerduty-cli services update`** - Update an existing service.

A service may represent an application, component, or team you wish to open incidents against.

There is a limit of 100,000 open Incidents per Service. If the limit is reached and you disable `auto_resolve_timeout` (set to 0 or null), the API will respond with an error.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `services.write`
- **`pagerduty-cli services update-custom-field`** - Update a Custom Field for Services.

Scoped OAuth requires: `custom_fields.write`
- **`pagerduty-cli services update-custom-field-option`** - Update a field option for a given field.

Scoped OAuth requires: `custom_fields.write`

### session-configurations

Manage session configurations

- **`pagerduty-cli session-configurations delete`** - Deletes the session configurations for a PagerDuty account that was previously set.
The type parameter is required and specifies which configurations to delete.
A single type ('mobile' or 'web') or comma-separated list may be passed in.

Scoped OAuth requires: `session_configurations.write`
- **`pagerduty-cli session-configurations get`** - Retrieves session configurations for a PagerDuty account. Returns an array containing
the requested configurations. If a specific type is requested, the array contains one item.
If no type is specified, the array contains all available configurations (mobile and web).
If no configurations exist, a 404 Not Found error will be returned.

A Session Configuration needs to be created before it can be retrieved and used.

Scoped OAuth requires: `session_configurations.read`
- **`pagerduty-cli session-configurations update`** - Creates or updates session configurations for a PagerDuty Account. The configurations will take effect immediately for new sessions, while existing sessions for the specified `types` are immediately revoked.

Scoped OAuth requires: `session_configurations.write`

### sre-agent

The SRE Agent uses AI to help manage and resolve incidents. Memories are knowledge learned by the SRE Agent from past incidents and conversations.

- **`pagerduty-cli sre-agent delete-sre-memory`** - Permanently delete an SRE Agent memory.

Scoped OAuth requires: `sre_agent.write`
- **`pagerduty-cli sre-agent list-sre-memories`** - Search SRE Agent memories for the account.

Memories are knowledge learned by the SRE Agent, including service runbooks, service profiles,
incident playbooks, and incident summaries. Filter by service ID, incident ID, or memory type to retrieve
relevant memories.

Scoped OAuth requires: `incident.read`
- **`pagerduty-cli sre-agent update-sre-memory`** - Update an existing SRE Agent memory.

Scoped OAuth requires: `sre_agent.write`

### standards

Standards help provide a clear understanding of what a good service configuration looks like, allowing to share and enforce organization guidelines across services to ensure adherence to best practices.

- **`pagerduty-cli standards list`** - Get all standards of an account.

Scoped OAuth requires: `standards.read`
- **`pagerduty-cli standards list-resource`** - List standards applied to a specific resource

Scoped OAuth requires: `standards.read`
- **`pagerduty-cli standards list-resource-many-services`** - List standards applied to a set of resources

Scoped OAuth requires: `standards.read`
- **`pagerduty-cli standards update`** - Updates a standard

Scoped OAuth requires: `standards.write`

### status-dashboards

Status Dashboards represent user-defined views for the Status Dashboard product that are limited to specific Business Services rather than the whole set of top-level Business Services (those with no dependent Services).

- **`pagerduty-cli status-dashboards get-by-id`** - Get a Status Dashboard by its PagerDuty `id`.

Scoped OAuth requires: `status_dashboards.read`
- **`pagerduty-cli status-dashboards get-by-url-slug`** - Get a Status Dashboard by its PagerDuty `url_slug`.  A `url_slug` is a human-readable reference
for a custom Status Dashboard that may be created or changed in the UI. It will generally be a `dash-separated-string-like-this`.

Scoped OAuth requires: `status_dashboards.read`
- **`pagerduty-cli status-dashboards get-service-impacts-by-url-slug`** - Get Business Service Impacts for the Business Services on a Status Dashboard by its `url_slug`. A `url_slug` is a human-readable reference
for a custom Status Dashboard that may be created or changed in the UI. It will generally be a `dash-separated-string-like-this`.

This endpoint does not return an exhaustive list of Business Services but rather provides access to the most impacted on the Status Dashboard up to the limit of 200.

The returned Business Services are sorted first by Impact, secondarily by most recently impacted, and finally by name.

To get impact information about a specific Business Service on the Status Dashboard that does not appear in the Impact-sored response, use the `ids[]` parameter on the `/business_services/impacts` endpoint.

Scoped OAuth requires: `status_dashboards.read`
- **`pagerduty-cli status-dashboards list`** - Get all your account's custom Status Dashboard views.

Scoped OAuth requires: `status_dashboards.read`

### status-pages

Status Pages can be public or private read-only pages, that display the status of some predefined set of services, to be shared with customers or internal stakeholders.

- **`pagerduty-cli status-pages`** - List Status Pages.

Scoped OAuth requires: `status_pages.read`

### tags

A Tag is applied to Escalation Policies, Teams or Users and can be used to filter them.

- **`pagerduty-cli tags create`** - Create a Tag.

A Tag is applied to Escalation Policies, Teams or Users and can be used to filter them.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `tags.write`
- **`pagerduty-cli tags delete`** - Remove an existing Tag.

A Tag is applied to Escalation Policies, Teams or Users and can be used to filter them.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `tags.write`
- **`pagerduty-cli tags get`** - Get details about an existing Tag.

A Tag is applied to Escalation Policies, Teams or Users and can be used to filter them.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `tags.read`
- **`pagerduty-cli tags get-by-entity-type`** - Get related Users, Teams or Escalation Policies for the Tag.

A Tag is applied to Escalation Policies, Teams or Users and can be used to filter them.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `tags.read`
- **`pagerduty-cli tags list`** - List all of your account's tags.

A Tag is applied to Escalation Policies, Teams or Users and can be used to filter them.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `tags.read`

### teams

A team is a collection of Users and Escalation Policies that represent a group of people within an organization.

- **`pagerduty-cli teams create`** - Create a new Team.

A team is a collection of Users and Escalation Policies that represent a group of people within an organization.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `teams.write`
- **`pagerduty-cli teams delete`** - Remove an existing team.

Succeeds only if the team has no associated Escalation Policies, Services, Schedules and Subteams.

All associated unresovled incidents will be reassigned to another team (if specified) or will loose team association, thus becoming account-level (with visibility implications).

Note that the incidents reassignment process is asynchronous and has no guarantee to complete before the API call return.

A team is a collection of Users and Escalation Policies that represent a group of people within an organization.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `teams.write`
- **`pagerduty-cli teams get`** - Get details about an existing team.

A team is a collection of Users and Escalation Policies that represent a group of people within an organization.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `teams.read`
- **`pagerduty-cli teams list`** - List teams of your PagerDuty account, optionally filtered by a search query.

A team is a collection of Users and Escalation Policies that represent a group of people within an organization.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `teams.read`
- **`pagerduty-cli teams update`** - Update an existing team.

A team is a collection of Users and Escalation Policies that represent a group of people within an organization.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `teams.write`

### templates

Templates is a new feature which will allow customers to create message templates to be leveraged by (but not limited to) status updates. The API will be secured to customers with the status updates entitlements.

- **`pagerduty-cli templates create`** - Create a new template

Scoped OAuth requires: `templates.write`
- **`pagerduty-cli templates delete`** - Delete a specific of templates on the account

Scoped OAuth requires: `templates.write`
- **`pagerduty-cli templates get`** - Get a list of all the template on an account

Scoped OAuth requires: `templates.read`
- **`pagerduty-cli templates get-fields`** - Get a list of fields that can be used on the account templates.

Scoped OAuth requires: `templates.read`
- **`pagerduty-cli templates get-id`** - Get a single template on the account

Scoped OAuth requires: `templates.read`
- **`pagerduty-cli templates update`** - Update an existing template

Scoped OAuth requires: `templates.write`

### users

Users are members of a PagerDuty account that have the ability to interact with Incidents and other data on the account.

- **`pagerduty-cli users create`** - Create a new user.

Users are members of a PagerDuty account that have the ability to interact with Incidents and other data on the account.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `users.write`
- **`pagerduty-cli users delete`** - Remove an existing user.

Returns 400 if the user has assigned incidents unless your [pricing plan](https://www.pagerduty.com/pricing) has the `offboarding` feature and the account is [configured](https://support.pagerduty.com/docs/offboarding#section-additional-configurations) appropriately.

Note that the incidents reassignment process is asynchronous and has no guarantee to complete before the api call return.

[*Learn more about `offboarding` feature*](https://support.pagerduty.com/docs/offboarding).

Users are members of a PagerDuty account that have the ability to interact with Incidents and other data on the account.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `users.write`
- **`pagerduty-cli users get`** - Get details about an existing user.

Users are members of a PagerDuty account that have the ability to interact with Incidents and other data on the account.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `users.read`
- **`pagerduty-cli users get-current`** - Get details about the current user.

This endpoint can only be used with a [user-level API key](https://support.pagerduty.com/docs/using-the-api#section-generating-a-personal-rest-api-key) or a key generated through an OAuth flow. This will not work if the request is made with an account-level access token.

Users are members of a PagerDuty account that have the ability to interact with Incidents and other data on the account.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)
- **`pagerduty-cli users list`** - List users of your PagerDuty account, optionally filtered by a search query.

Users are members of a PagerDuty account that have the ability to interact with Incidents and other data on the account.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `users.read`
- **`pagerduty-cli users update`** - Update an existing user.

Users are members of a PagerDuty account that have the ability to interact with Incidents and other data on the account.

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `users.write`

### vendors

A PagerDuty Vendor represents a specific type of integration. AWS Cloudwatch, Splunk, Datadog are all examples of vendors

- **`pagerduty-cli vendors get`** - Get details about one specific vendor.

A PagerDuty Vendor represents a specific type of integration. AWS Cloudwatch, Splunk, Datadog are all examples of vendors

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `vendors.read`
- **`pagerduty-cli vendors list`** - List all vendors.

A PagerDuty Vendor represents a specific type of integration. AWS Cloudwatch, Splunk, Datadog are all examples of vendors

For more information see the [API Concepts Document](https://developer.pagerduty.com/api-reference/)

Scoped OAuth requires: `vendors.read`

### webhook-subscriptions

Manage webhook subscriptions

- **`pagerduty-cli webhook-subscriptions create`** - Creates a new webhook subscription.

For more information on webhook subscriptions and how they are used to configure v3 webhooks
see the [Webhooks v3 Developer Documentation](https://developer.pagerduty.com/docs/webhooks/v3-overview/).

Scoped OAuth requires: `webhook_subscriptions.write`
- **`pagerduty-cli webhook-subscriptions create-oauth-client`** - Create a new OAuth client for webhook subscriptions. The client credentials will be validated by attempting to obtain an access token before creation.

Requires admin or owner role permissions.

Maximum of 10 OAuth clients per account.
- **`pagerduty-cli webhook-subscriptions delete`** - Deletes a webhook subscription.

Scoped OAuth requires: `webhook_subscriptions.write`
- **`pagerduty-cli webhook-subscriptions delete-oauth-client`** - Delete an OAuth client. This will also remove the OAuth client association from any webhook subscriptions using it.

Requires admin or owner role permissions.
- **`pagerduty-cli webhook-subscriptions get`** - Gets details about an existing webhook subscription.

Scoped OAuth requires: `webhook_subscriptions.read`
- **`pagerduty-cli webhook-subscriptions get-oauth-client`** - Get details of a specific OAuth client by ID.

Requires admin or owner role permissions.
- **`pagerduty-cli webhook-subscriptions list`** - List existing webhook subscriptions.

The `filter_type` and `filter_id` query parameters may be used to only show subscriptions
for a particular _service_ or _team_.

For more information on webhook subscriptions and how they are used to configure v3 webhooks
see the [Webhooks v3 Developer Documentation](https://developer.pagerduty.com/docs/webhooks/v3-overview/).

Scoped OAuth requires: `webhook_subscriptions.read`
- **`pagerduty-cli webhook-subscriptions list-oauth-clients`** - List all OAuth clients for webhook subscriptions. Maximum of 10 clients per account.

Requires admin or owner role permissions.
- **`pagerduty-cli webhook-subscriptions update`** - Updates an existing webhook subscription.

Only the fields being updated need to be included on the request.  This operation does not
support updating the `delivery_method` of the webhook subscription.

Scoped OAuth requires: `webhook_subscriptions.write`
- **`pagerduty-cli webhook-subscriptions update-oauth-client`** - Update an existing OAuth client. Any change will trigger token validation with the OAuth server.

Requires admin or owner role permissions.

### workflows

Manage workflows

- **`pagerduty-cli workflows create-integration-connection`** - Create a new Workflow Integration Connection.

Scoped OAuth requires: `workflow_integrations:connections.write`
- **`pagerduty-cli workflows delete-integration-connection`** - Delete a Workflow Integration Connection.

Scoped OAuth requires: `workflow_integrations:connections.write`
- **`pagerduty-cli workflows get-integration`** - Get details about a Workflow Integration.

Scoped OAuth requires: `workflow_integrations.read`
- **`pagerduty-cli workflows get-integration-connection`** - Get details about a Workflow Integration Connection.

Scoped OAuth requires: `workflow_integrations:connections.read`
- **`pagerduty-cli workflows list-integration-connections`** - List all Workflow Integration Connections.

Scoped OAuth requires: `workflow_integrations:connections.read`
- **`pagerduty-cli workflows list-integration-connections-by-integration`** - List all Workflow Integration Connections for a specific Workflow Integration.

Scoped OAuth requires: `workflow_integrations:connections.read`
- **`pagerduty-cli workflows list-integrations`** - List available Workflow Integrations.

Scoped OAuth requires: `workflow_integrations.read`
- **`pagerduty-cli workflows update-integration-connection`** - Update an existing Workflow Integration Connection.

Scoped OAuth requires: `workflow_integrations:connections.write`


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
pagerduty-cli abilities list

# JSON for scripting and agents
pagerduty-cli abilities list --json

# Filter to specific fields
pagerduty-cli abilities list --json --select id,name,status

# Dry run  -  show the request without sending
pagerduty-cli abilities list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
pagerduty-cli abilities list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and `--ignore-missing` to delete retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
pagerduty-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/pagerduty-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `PAGERDUTY_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `pagerduty-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `pagerduty-cli doctor` to check credentials
- Verify the environment variable is set: `echo $PAGERDUTY_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call**  -  Set PAGERDUTY_API_KEY to a valid REST API key; the header must be `Token token=<key>` (the CLI adds the prefix for you).
- **429 rate limited**  -  PagerDuty caps REST at ~960 req/min/account; the client backs off on Retry-After automatically  -  narrow --since/--until or --limit to reduce calls.
- **pulse or insights returns nothing**  -  Run `pagerduty-cli sync` first; the analytics commands read the local store, not the live API.
- **oncall who shows no next person**  -  The service's escalation policy may have a single tier or an empty schedule  -  run `pagerduty-cli audit coverage` to find the gap.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**pagerduty-mcp-server**](https://github.com/PagerDuty/pagerduty-mcp-server)  -  Python
- [**pagerduty-cli**](https://github.com/martindstone/pagerduty-cli)  -  TypeScript
- [**go-pagerduty**](https://github.com/PagerDuty/go-pagerduty)  -  Go
- [**python-pagerduty**](https://github.com/PagerDuty/python-pagerduty)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
