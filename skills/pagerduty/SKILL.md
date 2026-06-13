---
name: pagerduty
description: "Every PagerDuty incident, on-call and service operation from the terminal, plus a local SQLite mirror that answers cross-entity questions  -  MTTA/MTTR, on-call coverage gaps, responder load  -  that neither the API nor the web UI can. Trigger phrases: `who is on call for this service`, `show me the open pagerduty incidents`, `what's the mttr for this service`, `acknowledge the pagerduty incident`, `which services have no on-call coverage`, `use pagerduty`, `run pagerduty-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "PagerDuty"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - pagerduty-cli
    install:
      - kind: go
        bins: [pagerduty-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/pagerduty/cmd/pagerduty-cli
---

# PagerDuty  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `pagerduty-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install pagerduty --cli-only
   ```
2. Verify: `pagerduty-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/pagerduty/cmd/pagerduty-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Triage the incident queue, resolve who's on call now and next, and run service and escalation hygiene checks without leaving the shell. Sync once and the local store powers analytics no single API call exposes: pulse for what's hot right now, oncall who for the live escalation chain, audit coverage for escalation gaps, and insights mttr/responders/noisy for offline post-incident analytics.

## When to Use This CLI

Choose this CLI when an agent or operator needs to manage PagerDuty incidents, on-call schedules, services and escalation policies from the terminal, or when they need cross-entity analytics (MTTA/MTTR, on-call coverage, responder workload, noisy services) that the live API exposes only through the paid Analytics product or not at all. It is the right tool for MSP NOC triage, on-call resolution under pressure, and monthly reliability reviews.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI to send or ingest events/alerts (triggering incidents from monitoring)  -  that is the PagerDuty Events API v2, a separate ingestion surface this CLI does not wrap.
- Do not use it for real-time push notifications or webhook delivery  -  it polls REST; use PagerDuty webhook subscriptions for push.
- Do not treat insights mttr/responders/noisy as a replacement for paid PagerDuty Analytics in billing disputes  -  they are offline reconstructions from synced log entries and only as complete as the last sync window.
- Do not use it to administer users/SSO/billing at the account level  -  account provisioning belongs in the PagerDuty web admin console.

## Unique Capabilities

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

## Command Reference

**abilities**  -  This describes your account's abilities by feature name. For example `"teams"`.
An ability may be available to your account based on things like your pricing plan or account state.

- `pagerduty-cli abilities get-ability`  -  Test whether your account has a given ability. 'Abilities' describes your account's capabilities by feature name.
- `pagerduty-cli abilities list`  -  List all of your account's abilities, by name. 'Abilities' describes your account's capabilities by feature name.

**addons**  -  Manage addons

- `pagerduty-cli addons create`  -  Install an Add-on for your account.
- `pagerduty-cli addons delete`  -  Remove an existing Add-on.
- `pagerduty-cli addons get`  -  Get details about an existing Add-on.
- `pagerduty-cli addons list`  -  List all of the Add-ons installed on your account.
- `pagerduty-cli addons update`  -  Update an existing Add-on.

**alert-grouping-settings**  -  Alert Grouping Settings allow you to configure how alerts in services are grouped together into incidents.

- `pagerduty-cli alert-grouping-settings delete`  -  Delete an existing Alert Grouping Setting.
- `pagerduty-cli alert-grouping-settings get`  -  Get an existing Alert Grouping Setting.
- `pagerduty-cli alert-grouping-settings list`  -  List all of your alert grouping settings including both single service settings and global content based settings.
- `pagerduty-cli alert-grouping-settings post`  -  Create a new Alert Grouping Setting.
- `pagerduty-cli alert-grouping-settings put`  -  Update an Alert Grouping Setting.

**audit**  -  Provides audit record data.

- `pagerduty-cli audit`  -  List audit trail records matching provided query params or default criteria.

**automation-actions**  -  Automation Actions invoke jobs that are staged in Runbook Automation or Process Automation.

- `pagerduty-cli automation-actions create`  -  Create a Script, Process Automation, or Runbook Automation action
- `pagerduty-cli automation-actions create-invocation`  -  Create an Invocation
- `pagerduty-cli automation-actions create-runner`  -  Create a Process Automation or a Runbook Automation runner.
- `pagerduty-cli automation-actions create-runner-team-association`  -  Associate a runner with a team
- `pagerduty-cli automation-actions create-service-assocation`  -  Associate an Automation Action with a service
- `pagerduty-cli automation-actions create-team-association`  -  Associate an Automation Action with a team
- `pagerduty-cli automation-actions delete`  -  Delete an Automation Action
- `pagerduty-cli automation-actions delete-runner`  -  Delete an Automation Action runner
- `pagerduty-cli automation-actions delete-runner-team-association`  -  Disassociates a runner from a team
- `pagerduty-cli automation-actions delete-service-association`  -  Disassociate an Automation Action from a service
- `pagerduty-cli automation-actions delete-team-association`  -  Disassociate an Automation Action from a team
- `pagerduty-cli automation-actions get`  -  Get an Automation Action
- `pagerduty-cli automation-actions get-action-service-association`  -  Gets the details of a Automation Action / service relation
- `pagerduty-cli automation-actions get-action-service-associations`  -  Gets all service references associated with an Automation Action
- `pagerduty-cli automation-actions get-action-team-association`  -  Gets the details of an Automation Action / team relation
- `pagerduty-cli automation-actions get-action-team-associations`  -  Gets all team references associated with an Automation Action
- `pagerduty-cli automation-actions get-all`  -  Lists Automation Actions matching provided query params.
- `pagerduty-cli automation-actions get-invocation`  -  Get an Automation Action Invocation
- `pagerduty-cli automation-actions get-runner`  -  Get an Automation Action runner
- `pagerduty-cli automation-actions get-runner-team-association`  -  Gets the details of a runner / team relation
- `pagerduty-cli automation-actions get-runner-team-associations`  -  Gets all team references associated with a runner
- `pagerduty-cli automation-actions get-runners`  -  Lists Automation Action runners matching provided query params.
- `pagerduty-cli automation-actions list-invocations`  -  List Invocations
- `pagerduty-cli automation-actions update`  -  Updates an Automation Action
- `pagerduty-cli automation-actions update-runner`  -  Update an Automation Action runner

**business-services**  -  Business services model capabilities that span multiple technical services and that may be owned by several different teams.

- `pagerduty-cli business-services create`  -  Create a new Business Service.
- `pagerduty-cli business-services delete`  -  Delete an existing Business Service.
- `pagerduty-cli business-services delete-priority-thresholds`  -  Clears the Priority Threshold for the account.
- `pagerduty-cli business-services get`  -  Get details about an existing Business Service.
- `pagerduty-cli business-services get-impacts`  -  Retrieve a list top-level Business Services sorted by highest Impact with `status` included.
- `pagerduty-cli business-services get-priority-thresholds`  -  Retrieves the priority threshold information for an account.
- `pagerduty-cli business-services get-top-level-impactors`  -  Retrieve a list of Impactors for the top-level Business Services on the account.
- `pagerduty-cli business-services list`  -  List existing Business Services.
- `pagerduty-cli business-services put-priority-thresholds`  -  Set the Account-level priority threshold for Business Service. Scoped OAuth requires: `services.write`
- `pagerduty-cli business-services update`  -  Update an existing Business Service. NOTE that this endpoint also accepts the PATCH verb.

**change-events**  -  Change Events enable you to send informational events about recent changes such as code deploys and system config changes from any system that can make an outbound HTTP connection. These events do not create incidents and do not send notifications; they are shown in context with incidents on the same PagerDuty service.

- `pagerduty-cli change-events create`  -  Sending Change Events is documented as part of the V2 Events API. See [`Send Change Event`](https://developer.pagerduty.
- `pagerduty-cli change-events get`  -  Get details about an existing Change Event. Scoped OAuth requires: `change_events.read`
- `pagerduty-cli change-events list`  -  List all of the existing Change Events. Scoped OAuth requires: `change_events.read`
- `pagerduty-cli change-events update`  -  Update an existing Change Event Scoped OAuth requires: `change_events.write`

**escalation-policies**  -  Escalation policies define which user should be alerted at which time.

- `pagerduty-cli escalation-policies create-escalation-policy`  -  Creates a new escalation policy. At least one escalation rule must be provided.
- `pagerduty-cli escalation-policies delete-escalation-policy`  -  Deletes an existing escalation policy and rules. The escalation policy must not be in use by any services.
- `pagerduty-cli escalation-policies get-escalation-policy`  -  Get information about an existing escalation policy and its rules.
- `pagerduty-cli escalation-policies list`  -  List all of the existing escalation policies. Escalation policies define which user should be alerted at which time.
- `pagerduty-cli escalation-policies update-escalation-policy`  -  Updates an existing escalation policy and rules. Escalation policies define which user should be alerted at which time.

**event-orchestrations**  -  Event Orchestrations allow you to route events to an endpoint and create collections of Event Orchestrations, which define sets of actions to take based on event content.

- `pagerduty-cli event-orchestrations create-cache-var-on-service-orch`  -  Create a Cache Variable for a Service Event Orchestration.
- `pagerduty-cli event-orchestrations delete-cache-var-on-service-orch`  -  Delete a Cache Variable for a Service Event Orchestration.
- `pagerduty-cli event-orchestrations delete-orchestration`  -  Delete a Global Event Orchestration.
- `pagerduty-cli event-orchestrations get-cache-var-on-service-orch`  -  Get a Cache Variable for a Service Event Orchestration.
- `pagerduty-cli event-orchestrations get-orch-active-status`  -  Get a Service Orchestration's active status.
- `pagerduty-cli event-orchestrations get-orch-path-service`  -  Get a Service Orchestration. A Service Orchestration allows you to create a set of Event Rules.
- `pagerduty-cli event-orchestrations get-orchestration`  -  Get a Global Event Orchestration.
- `pagerduty-cli event-orchestrations list`  -  List all Global Event Orchestrations on an Account.
- `pagerduty-cli event-orchestrations list-cache-var-on-service-orch`  -  List Cache Variables for a Service Event Orchestration.
- `pagerduty-cli event-orchestrations post-orchestration`  -  Create a Global Event Orchestration.
- `pagerduty-cli event-orchestrations update-cache-var-on-service-orch`  -  Update a Cache Variable for a Service Event Orchestration.
- `pagerduty-cli event-orchestrations update-orch-active-status`  -  Update a Service Orchestration's active status.
- `pagerduty-cli event-orchestrations update-orch-path-service`  -  Update a Service Orchestration. A Service Orchestration allows you to create a set of Event Rules.
- `pagerduty-cli event-orchestrations update-orchestration`  -  Update a Global Event Orchestration.

**extension-schemas**  -  A PagerDuty extension vendor represents a specific type of outbound extension such as Generic Webhook, Slack, ServiceNow.

- `pagerduty-cli extension-schemas get`  -  Get details about one specific extension vendor.
- `pagerduty-cli extension-schemas list`  -  List all extension schemas.

**extensions**  -  Extensions are representations of Extension Schema objects that are attached to Services.

- `pagerduty-cli extensions create`  -  Create a new Extension. Extensions are representations of Extension Schema objects that are attached to Services.
- `pagerduty-cli extensions delete`  -  Delete an existing extension.
- `pagerduty-cli extensions get`  -  Get details about an existing extension.
- `pagerduty-cli extensions list`  -  List existing extensions. Extensions are representations of Extension Schema objects that are attached to Services.
- `pagerduty-cli extensions update`  -  Update an existing extension. Extensions are representations of Extension Schema objects that are attached to Services.

**incident-workflows**  -  An Incident Workflow is a sequence of configurable Steps and associated Triggers that can execute automated Actions for a given Incident.

- `pagerduty-cli incident-workflows associate-service-to-trigger`  -  Associate a Service with an existing Incident Workflow Trigger Scoped OAuth requires: `incident_workflows.write`
- `pagerduty-cli incident-workflows create-trigger`  -  Create new Incident Workflow Trigger Scoped OAuth requires: `incident_workflows.write`
- `pagerduty-cli incident-workflows delete`  -  Delete an existing Incident Workflow An Incident Workflow is a sequence of configurable Steps and associated Triggers
- `pagerduty-cli incident-workflows delete-service-from-trigger`  -  Remove a an existing Service from an Incident Workflow Trigger Scoped OAuth requires: `incident_workflows.write`
- `pagerduty-cli incident-workflows delete-trigger`  -  Delete an existing Incident Workflow Trigger Scoped OAuth requires: `incident_workflows.write`
- `pagerduty-cli incident-workflows get`  -  Get an existing Incident Workflow An Incident Workflow is a sequence of configurable Steps and associated Triggers that
- `pagerduty-cli incident-workflows get-action`  -  Get an Incident Workflow Action Scoped OAuth requires: `incident_workflows.read`
- `pagerduty-cli incident-workflows get-trigger`  -  Retrieve an existing Incident Workflows Trigger Scoped OAuth requires: `incident_workflows.read`
- `pagerduty-cli incident-workflows list`  -  List existing Incident Workflows. This is the best method to use to list all Incident Workflows in your account.
- `pagerduty-cli incident-workflows list-actions`  -  List Incident Workflow Actions Scoped OAuth requires: `incident_workflows.read`
- `pagerduty-cli incident-workflows list-triggers`  -  List existing Incident Workflow Triggers Scoped OAuth requires: `incident_workflows.read`
- `pagerduty-cli incident-workflows post`  -  Create a new Incident Workflow An Incident Workflow is a sequence of configurable Steps and associated Triggers that
- `pagerduty-cli incident-workflows put`  -  Update an Incident Workflow An Incident Workflow is a sequence of configurable Steps and associated Triggers that can
- `pagerduty-cli incident-workflows update-trigger`  -  Update an existing Incident Workflow Trigger Scoped OAuth requires: `incident_workflows.write`

**incidents**  -  An incident represents a problem or an issue that needs to be addressed and resolved. Incidents trigger on a service, which prompts notifications to go out to on-call responders per the service's escalation policy.

- `pagerduty-cli incidents create`  -  Create an incident synchronously without a corresponding event from a monitoring service.
- `pagerduty-cli incidents create-custom-fields-field`  -  <!
- `pagerduty-cli incidents create-custom-fields-field-option`  -  <!
- `pagerduty-cli incidents create-type`  -  Create a new incident type.
- `pagerduty-cli incidents create-type-custom-field`  -  Create a Custom Field for an Incident Type Custom Fields (CF)
- `pagerduty-cli incidents create-type-custom-field-field-options`  -  Create a field option for a custom field.
- `pagerduty-cli incidents delete-custom-fields-field`  -  <!
- `pagerduty-cli incidents delete-custom-fields-field-option`  -  <!
- `pagerduty-cli incidents delete-type-custom-field`  -  Delete a custom field for an incident type.
- `pagerduty-cli incidents delete-type-custom-field-field-option`  -  Delete a field option for a custom field.
- `pagerduty-cli incidents get`  -  Show detailed information about an incident. Accepts either an incident id, or an incident number.
- `pagerduty-cli incidents get-custom-fields-field`  -  <!
- `pagerduty-cli incidents get-type`  -  Get detailed information about a single incident type. Accepts either an incident type id, or an incident type name.
- `pagerduty-cli incidents get-type-custom-field`  -  Get a custom field for an incident type.
- `pagerduty-cli incidents get-type-custom-field-field-options`  -  Get a field option on a custom field Custom Fields (CF)
- `pagerduty-cli incidents list`  -  List existing incidents. An incident represents a problem or an issue that needs to be addressed and resolved.
- `pagerduty-cli incidents list-custom-fields-field-options`  -  <!
- `pagerduty-cli incidents list-custom-fields-fields`  -  <!
- `pagerduty-cli incidents list-type-custom-field`  -  List field options for a custom field.
- `pagerduty-cli incidents list-type-custom-fields`  -  List the custom fields for an incident type.
- `pagerduty-cli incidents list-types`  -  List the available incident types Incident Types are a feature which will allow customers to categorize incidents
- `pagerduty-cli incidents update`  -  Acknowledge, resolve, escalate or reassign one or more incidents.
- `pagerduty-cli incidents update-custom-fields-field`  -  <!
- `pagerduty-cli incidents update-custom-fields-field-option`  -  <!
- `pagerduty-cli incidents update-id`  -  Acknowledge, resolve, escalate or reassign an incident.
- `pagerduty-cli incidents update-type`  -  Update an Incident Type.
- `pagerduty-cli incidents update-type-custom-field`  -  Update a custom field for an incident type. Field Options can also be updated within the same call.
- `pagerduty-cli incidents update-type-custom-field-field-option`  -  Update a field option for a custom field.

**ip-allow-lists**  -  Manage account-level IP Allow Lists that restrict access to your PagerDuty subdomain to a configured set of IPv4 CIDR ranges. Enforcement currently applies to web and mobile application traffic.


<!-- theme: warning -->

> ### Early Access
> This API is in Early Access and may change at any time. You must pass the `X-EARLY-ACCESS: ip-allow-lists` header on every request, and your account must be enrolled in the IP Allow Lists Early Access program. Contact your PagerDuty account team to request access.

- `pagerduty-cli ip-allow-lists create`  -  <!-- theme: warning --> > ### Early Access > This API is in Early Access and may change at any time.
- `pagerduty-cli ip-allow-lists delete`  -  <!-- theme: warning --> > ### Early Access > This API is in Early Access and may change at any time.
- `pagerduty-cli ip-allow-lists get`  -  <!-- theme: warning --> > ### Early Access > This API is in Early Access and may change at any time.
- `pagerduty-cli ip-allow-lists list`  -  <!-- theme: warning --> > ### Early Access > This API is in Early Access and may change at any time.
- `pagerduty-cli ip-allow-lists update`  -  <!-- theme: warning --> > ### Early Access > This API is in Early Access and may change at any time.

**license-allocations**  -  Manage license allocations

- `pagerduty-cli license-allocations`  -  List the Licenses allocated to Users within your Account Scoped OAuth requires: `licenses.read`

**licenses**  -  Licenses are allocated to Users to allow for per-User access to PagerDuty functionality within an Account.

- `pagerduty-cli licenses`  -  List the Licenses associated with your Account Scoped OAuth requires: `licenses.read`

**log-entries**  -  A log of all the events that happen to an Incident, and these are exposed as Log Entries.

- `pagerduty-cli log-entries get-log-entry`  -  Get details for a specific incident log entry.
- `pagerduty-cli log-entries list`  -  List all of the incident log entries across the entire account.

**maintenance-windows**  -  A Maintenance Window is used to temporarily disable one or more Services for a set period of time.

- `pagerduty-cli maintenance-windows create`  -  Create a new maintenance window for the specified services.
- `pagerduty-cli maintenance-windows delete`  -  Delete an existing maintenance window if it's in the future, or end it if it's currently on-going.
- `pagerduty-cli maintenance-windows get`  -  Get an existing maintenance window.
- `pagerduty-cli maintenance-windows list`  -  List existing maintenance windows, optionally filtered by service and/or team, or whether they are from the past
- `pagerduty-cli maintenance-windows update`  -  Update an existing maintenance window.

**notifications**  -  A Notification is created when an Incident is triggered or escalated.

- `pagerduty-cli notifications`  -  List notifications for a given time range, optionally filtered by type (sms_notification, email_notification

**oauth-delegations**  -  An OAuth Delegation represents a delegation of a User's permissions to an OAuth Client, allowing the client to impersonate the user when making API requests.

- `pagerduty-cli oauth-delegations delete`  -  Delete all OAuth delegations as per provided query parameters.
- `pagerduty-cli oauth-delegations get-revocation-requests-status`  -  <!-- theme: warning --> > ### Deprecated > This endpoint is deprecated as OAuth token revocation is now synchronous.

**oncalls**  -  Manage oncalls

- `pagerduty-cli oncalls`  -  List the on-call entries during a given time range.

**pagerduty-analytics**  -  Manage pagerduty analytics

- `pagerduty-cli pagerduty-analytics get-incident-responses-by-id`  -  Provides enriched responder data for a single incident.
- `pagerduty-cli pagerduty-analytics get-incidents`  -  Provides enriched incident data and metrics for multiple incidents.
- `pagerduty-cli pagerduty-analytics get-incidents-by-id`  -  Provides enriched incident data and metrics for a single incident.
- `pagerduty-cli pagerduty-analytics get-metrics-incidents-all`  -  Provides aggregated enriched metrics for incidents.
- `pagerduty-cli pagerduty-analytics get-metrics-incidents-escalation-policy`  -  Provides aggregated metrics for incidents aggregated into units of time by escalation policy.
- `pagerduty-cli pagerduty-analytics get-metrics-incidents-escalation-policy-all`  -  Provides aggregated metrics across all escalation policies.
- `pagerduty-cli pagerduty-analytics get-metrics-incidents-service`  -  Provides aggregated metrics for incidents aggregated into units of time by service.
- `pagerduty-cli pagerduty-analytics get-metrics-incidents-service-all`  -  Provides aggregated metrics across all services.
- `pagerduty-cli pagerduty-analytics get-metrics-incidents-team`  -  Provides aggregated metrics for incidents aggregated into units of time by team.
- `pagerduty-cli pagerduty-analytics get-metrics-incidents-team-all`  -  Provides aggregated metrics across all teams.
- `pagerduty-cli pagerduty-analytics get-metrics-pd-advance-usage-features`  -  Provides aggregated metrics for the usage of PD Advance. <!
- `pagerduty-cli pagerduty-analytics get-metrics-responders-all`  -  Provides aggregated incident metrics for all selected responders.
- `pagerduty-cli pagerduty-analytics get-metrics-responders-team`  -  Provides incident metrics aggregated by responder.
- `pagerduty-cli pagerduty-analytics get-metrics-users-all`  -  Provides aggregated metrics across all users within their account.
- `pagerduty-cli pagerduty-analytics get-responder-incidents`  -  Provides enriched incident data and metrics for a specific responder.
- `pagerduty-cli pagerduty-analytics get-users`  -  Allows users to retrieve a raw list of user analytics data within their account.

**paused-incident-reports**  -  Provides paused Incident reporting data on services and accounts that have paused Alerts.

- `pagerduty-cli paused-incident-reports get-alerts`  -  Returns the 5 most recent alerts that were triggered after being paused and the 5 most recent alerts that were resolved
- `pagerduty-cli paused-incident-reports get-counts`  -  Returns reporting counts for paused Incident usage for a given reporting period (maximum 6 months lookback period).

**priorities**  -  A priority is a label representing the importance and impact of an incident. This feature is only available on Standard and Enterprise plans.

- `pagerduty-cli priorities`  -  List existing priorities, in order (most to least severe).

**rulesets**  -  Rulesets allow you to route events to an endpoint and create collections of Event Rules, which define sets of actions to take based on event content.

- `pagerduty-cli rulesets create`  -  Create a new Ruleset. <!-- theme: warning --> > ### End-of-life > Rulesets and Event Rules will end-of-life soon.
- `pagerduty-cli rulesets delete`  -  Delete a Ruleset. <!-- theme: warning --> > ### End-of-life > Rulesets and Event Rules will end-of-life soon.
- `pagerduty-cli rulesets get`  -  Get a Ruleset. <!-- theme: warning --> > ### End-of-life > Rulesets and Event Rules will end-of-life soon.
- `pagerduty-cli rulesets list`  -  List all Rulesets <!-- theme: warning --> > ### End-of-life > Rulesets and Event Rules will end-of-life soon.
- `pagerduty-cli rulesets update`  -  Update a Ruleset. <!-- theme: warning --> > ### End-of-life > Rulesets and Event Rules will end-of-life soon.

**schedules**  -  A Schedule determines the time periods that users are On-Call.

- `pagerduty-cli schedules create`  -  Create a new on-call schedule. A Schedule determines the time periods that users are On-Call.
- `pagerduty-cli schedules create-preview`  -  Preview what an on-call schedule would look like without saving it.
- `pagerduty-cli schedules create-v3`  -  <!
- `pagerduty-cli schedules delete`  -  Delete an on-call schedule. A Schedule determines the time periods that users are On-Call.
- `pagerduty-cli schedules delete-v3`  -  <!
- `pagerduty-cli schedules get`  -  Show detailed information about a schedule, including entries for each layer. Scoped OAuth requires: `schedules.read`
- `pagerduty-cli schedules get-v3`  -  <!
- `pagerduty-cli schedules list`  -  List the on-call schedules. A Schedule determines the time periods that users are On-Call.
- `pagerduty-cli schedules list-v3`  -  <!
- `pagerduty-cli schedules update`  -  Update an existing on-call schedule. A Schedule determines the time periods that users are On-Call.
- `pagerduty-cli schedules update-v3`  -  <!

**service-dependencies**  -  Services are categorized into technical and business services. Dependencies can be created via any combination of these services.

- `pagerduty-cli service-dependencies create-service-dependency`  -  Create new dependencies between two services.
- `pagerduty-cli service-dependencies delete-service-dependency`  -  Disassociate dependencies between two services.
- `pagerduty-cli service-dependencies get-business-service`  -  Get all immediate dependencies of any Business Service.
- `pagerduty-cli service-dependencies get-technical-service`  -  Get all immediate dependencies of any technical service. Technical services are also known as `services`.

**services**  -  A Service may represent an application, component, or team you wish to open incidents against.

- `pagerduty-cli services create`  -  Create a new service.
- `pagerduty-cli services create-custom-field`  -  Creates a new Custom Field for Services, along with the Field Options if provided.
- `pagerduty-cli services create-custom-field-option`  -  Create a new option for the given field. Scoped OAuth requires: `custom_fields.write`
- `pagerduty-cli services delete`  -  Delete an existing service.
- `pagerduty-cli services delete-custom-field`  -  Delete a Custom Field from Services. Scoped OAuth requires: `custom_fields.write`
- `pagerduty-cli services delete-custom-field-option`  -  Delete a field option. Scoped OAuth requires: `custom_fields.write`
- `pagerduty-cli services get`  -  Get details about an existing service.
- `pagerduty-cli services get-custom-field`  -  Show detailed information about a Custom Field for Services. Scoped OAuth requires: `custom_fields.read`
- `pagerduty-cli services get-custom-field-option`  -  Get a field option for a given field. Scoped OAuth requires: `custom_fields.read`
- `pagerduty-cli services list`  -  List existing Services. A service may represent an application, component, or team you wish to open incidents against.
- `pagerduty-cli services list-custom-field-options`  -  List all options for a given field. Scoped OAuth requires: `custom_fields.read`
- `pagerduty-cli services list-custom-fields`  -  List Custom Fields available for Services. Scoped OAuth requires: `custom_fields.read`
- `pagerduty-cli services update`  -  Update an existing service.
- `pagerduty-cli services update-custom-field`  -  Update a Custom Field for Services. Scoped OAuth requires: `custom_fields.write`
- `pagerduty-cli services update-custom-field-option`  -  Update a field option for a given field. Scoped OAuth requires: `custom_fields.write`

**session-configurations**  -  Manage session configurations

- `pagerduty-cli session-configurations delete`  -  Deletes the session configurations for a PagerDuty account that was previously set.
- `pagerduty-cli session-configurations get`  -  Retrieves session configurations for a PagerDuty account. Returns an array containing the requested configurations.
- `pagerduty-cli session-configurations update`  -  Creates or updates session configurations for a PagerDuty Account.

**sre-agent**  -  The SRE Agent uses AI to help manage and resolve incidents. Memories are knowledge learned by the SRE Agent from past incidents and conversations.

- `pagerduty-cli sre-agent delete-sre-memory`  -  Permanently delete an SRE Agent memory. Scoped OAuth requires: `sre_agent.write`
- `pagerduty-cli sre-agent list-sre-memories`  -  Search SRE Agent memories for the account.
- `pagerduty-cli sre-agent update-sre-memory`  -  Update an existing SRE Agent memory. Scoped OAuth requires: `sre_agent.write`

**standards**  -  Standards help provide a clear understanding of what a good service configuration looks like, allowing to share and enforce organization guidelines across services to ensure adherence to best practices.

- `pagerduty-cli standards list`  -  Get all standards of an account. Scoped OAuth requires: `standards.read`
- `pagerduty-cli standards list-resource`  -  List standards applied to a specific resource Scoped OAuth requires: `standards.read`
- `pagerduty-cli standards list-resource-many-services`  -  List standards applied to a set of resources Scoped OAuth requires: `standards.read`
- `pagerduty-cli standards update`  -  Updates a standard Scoped OAuth requires: `standards.write`

**status-dashboards**  -  Status Dashboards represent user-defined views for the Status Dashboard product that are limited to specific Business Services rather than the whole set of top-level Business Services (those with no dependent Services).

- `pagerduty-cli status-dashboards get-by-id`  -  Get a Status Dashboard by its PagerDuty `id`. Scoped OAuth requires: `status_dashboards.read`
- `pagerduty-cli status-dashboards get-by-url-slug`  -  Get a Status Dashboard by its PagerDuty `url_slug`.
- `pagerduty-cli status-dashboards get-service-impacts-by-url-slug`  -  Get Business Service Impacts for the Business Services on a Status Dashboard by its `url_slug`.
- `pagerduty-cli status-dashboards list`  -  Get all your account's custom Status Dashboard views. Scoped OAuth requires: `status_dashboards.read`

**status-pages**  -  Status Pages can be public or private read-only pages, that display the status of some predefined set of services, to be shared with customers or internal stakeholders.

- `pagerduty-cli status-pages`  -  List Status Pages. Scoped OAuth requires: `status_pages.read`

**tags**  -  A Tag is applied to Escalation Policies, Teams or Users and can be used to filter them.

- `pagerduty-cli tags create`  -  Create a Tag. A Tag is applied to Escalation Policies, Teams or Users and can be used to filter them.
- `pagerduty-cli tags delete`  -  Remove an existing Tag. A Tag is applied to Escalation Policies, Teams or Users and can be used to filter them.
- `pagerduty-cli tags get`  -  Get details about an existing Tag.
- `pagerduty-cli tags get-by-entity-type`  -  Get related Users, Teams or Escalation Policies for the Tag.
- `pagerduty-cli tags list`  -  List all of your account's tags. A Tag is applied to Escalation Policies, Teams or Users and can be used to filter them.

**teams**  -  A team is a collection of Users and Escalation Policies that represent a group of people within an organization.

- `pagerduty-cli teams create`  -  Create a new Team.
- `pagerduty-cli teams delete`  -  Remove an existing team.
- `pagerduty-cli teams get`  -  Get details about an existing team.
- `pagerduty-cli teams list`  -  List teams of your PagerDuty account, optionally filtered by a search query.
- `pagerduty-cli teams update`  -  Update an existing team.

**templates**  -  Templates is a new feature which will allow customers to create message templates to be leveraged by (but not limited to) status updates. The API will be secured to customers with the status updates entitlements.

- `pagerduty-cli templates create`  -  Create a new template Scoped OAuth requires: `templates.write`
- `pagerduty-cli templates delete`  -  Delete a specific of templates on the account Scoped OAuth requires: `templates.write`
- `pagerduty-cli templates get`  -  Get a list of all the template on an account Scoped OAuth requires: `templates.read`
- `pagerduty-cli templates get-fields`  -  Get a list of fields that can be used on the account templates. Scoped OAuth requires: `templates.read`
- `pagerduty-cli templates get-id`  -  Get a single template on the account Scoped OAuth requires: `templates.read`
- `pagerduty-cli templates update`  -  Update an existing template Scoped OAuth requires: `templates.write`

**users**  -  Users are members of a PagerDuty account that have the ability to interact with Incidents and other data on the account.

- `pagerduty-cli users create`  -  Create a new user.
- `pagerduty-cli users delete`  -  Remove an existing user. Returns 400 if the user has assigned incidents unless your [pricing plan](https://www.
- `pagerduty-cli users get`  -  Get details about an existing user.
- `pagerduty-cli users get-current`  -  Get details about the current user. This endpoint can only be used with a [user-level API key](https://support.
- `pagerduty-cli users list`  -  List users of your PagerDuty account, optionally filtered by a search query.
- `pagerduty-cli users update`  -  Update an existing user.

**vendors**  -  A PagerDuty Vendor represents a specific type of integration. AWS Cloudwatch, Splunk, Datadog are all examples of vendors

- `pagerduty-cli vendors get`  -  Get details about one specific vendor. A PagerDuty Vendor represents a specific type of integration.
- `pagerduty-cli vendors list`  -  List all vendors. A PagerDuty Vendor represents a specific type of integration.

**webhook-subscriptions**  -  Manage webhook subscriptions

- `pagerduty-cli webhook-subscriptions create`  -  Creates a new webhook subscription.
- `pagerduty-cli webhook-subscriptions create-oauth-client`  -  Create a new OAuth client for webhook subscriptions.
- `pagerduty-cli webhook-subscriptions delete`  -  Deletes a webhook subscription. Scoped OAuth requires: `webhook_subscriptions.write`
- `pagerduty-cli webhook-subscriptions delete-oauth-client`  -  Delete an OAuth client. This will also remove the OAuth client association from any webhook subscriptions using it.
- `pagerduty-cli webhook-subscriptions get`  -  Gets details about an existing webhook subscription. Scoped OAuth requires: `webhook_subscriptions.read`
- `pagerduty-cli webhook-subscriptions get-oauth-client`  -  Get details of a specific OAuth client by ID. Requires admin or owner role permissions.
- `pagerduty-cli webhook-subscriptions list`  -  List existing webhook subscriptions.
- `pagerduty-cli webhook-subscriptions list-oauth-clients`  -  List all OAuth clients for webhook subscriptions. Maximum of 10 clients per account.
- `pagerduty-cli webhook-subscriptions update`  -  Updates an existing webhook subscription. Only the fields being updated need to be included on the request.
- `pagerduty-cli webhook-subscriptions update-oauth-client`  -  Update an existing OAuth client. Any change will trigger token validation with the OAuth server.

**workflows**  -  Manage workflows

- `pagerduty-cli workflows create-integration-connection`  -  Create a new Workflow Integration Connection. Scoped OAuth requires: `workflow_integrations:connections.write`
- `pagerduty-cli workflows delete-integration-connection`  -  Delete a Workflow Integration Connection. Scoped OAuth requires: `workflow_integrations:connections.write`
- `pagerduty-cli workflows get-integration`  -  Get details about a Workflow Integration. Scoped OAuth requires: `workflow_integrations.read`
- `pagerduty-cli workflows get-integration-connection`  -  Get details about a Workflow Integration Connection. Scoped OAuth requires: `workflow_integrations:connections.read`
- `pagerduty-cli workflows list-integration-connections`  -  List all Workflow Integration Connections. Scoped OAuth requires: `workflow_integrations:connections.read`
- `pagerduty-cli workflows list-integration-connections-by-integration`  -  List all Workflow Integration Connections for a specific Workflow Integration.
- `pagerduty-cli workflows list-integrations`  -  List available Workflow Integrations. Scoped OAuth requires: `workflow_integrations.read`
- `pagerduty-cli workflows update-integration-connection`  -  Update an existing Workflow Integration Connection. Scoped OAuth requires: `workflow_integrations:connections.write`


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
pagerduty-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

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

## Auth Setup

Authenticate with a PagerDuty REST API key in the Authorization header as `Token token=<key>`. Set it via the PAGERDUTY_API_KEY environment variable. Read-only by default  -  mutating commands require explicit flags and support --dry-run.

Run `pagerduty-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  pagerduty-cli abilities list --agent --select id,name,status
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
pagerduty-cli feedback "the --since flag is inclusive but docs say exclusive"
pagerduty-cli feedback --stdin < notes.txt
pagerduty-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/pagerduty-cli/feedback.jsonl`. They are never POSTed unless `PAGERDUTY_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `PAGERDUTY_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
pagerduty-cli profile save briefing --json
pagerduty-cli --profile briefing abilities list
pagerduty-cli profile list --json
pagerduty-cli profile show briefing
pagerduty-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `pagerduty-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/pagerduty/cmd/pagerduty-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add pagerduty-mcp -- pagerduty-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which pagerduty-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   pagerduty-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `pagerduty-cli <command> --help`.
