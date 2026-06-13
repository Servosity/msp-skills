# Rootly CLI

**Every Rootly incident, alert, and on-call object as a typed command, with a local SQLite mirror for offline analytics.**

The official rootly-cli exposes a handful of resource groups; Rootly's API has ~98. This CLI types the entire surface, then adds what no Rootly tool has: a local SQLite mirror powering offline incident-similarity (related), solution-mining (fixed-last-time), MTTR/MTTA analytics (mttr), service scorecards (service-health), and cross-schedule coverage-gap detection (coverage-gaps)  -  the same agentic capabilities the AI-Labs MCP server reaches a remote service to compute, here for free in your terminal.

## Install

The recommended path installs both the `rootly-cli` binary and the `pp-rootly` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install rootly
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install rootly --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install rootly --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install rootly --agent claude-code
npx -y @mvanhorn/printing-press-library install rootly --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/rootly/cmd/rootly-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/rootly-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install rootly --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-rootly --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-rootly --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install rootly --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/rootly-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `ROOTLY_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/rootly/cmd/rootly-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "rootly": {
      "command": "rootly-mcp",
      "env": {
        "ROOTLY_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Set a Rootly API key as ROOTLY_API_KEY (the same variable the official rootly-cli uses; ROOTLY_API_TOKEN is also accepted). Keys come in Global, Team, and Personal scopes from your Rootly account settings; the CLI sends it as a Bearer token over JSON:API. No tenant or org id is needed  -  the key identifies the workspace. Run `rootly-cli doctor` to confirm the key is valid and the API is reachable.

## Quick Start

```bash
# Confirm ROOTLY_API_KEY is valid and api.rootly.com is reachable before anything else.
rootly-cli doctor

# Mirror incidents, alerts, services, schedules, and on-call data into the local SQLite store.
rootly-cli sync --full

# List incidents with only the fields an agent needs from the nested JSON:API payload.
rootly-cli incidents list --agent --select data.id,data.attributes.title,data.attributes.severity,data.attributes.status

# See who is on call across every schedule in one table.
rootly-cli oncall-now --agent

# Compute MTTR/MTTA by service for the last 30 days from the local mirror.
rootly-cli mttr --by service --since 30d --agent

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Incident intelligence (offline)
- **`related`**  -  Find the past incidents most similar to a given one, ranked, so you can see how this class of problem played out before.

  _Reach for this during triage when a symptom feels familiar  -  it surfaces prior incidents and their resolutions without a remote ML round-trip._

  ```bash
  rootly-cli related INC-1234 --agent --limit 5
  ```
- **`war-room`**  -  One screen for an active incident: header, severity/status, full timeline, open action items, the on-call for the affected service, and the next escalation rung.

  _Use at the top of an incident to get full situational context in one command instead of five browser tabs._

  ```bash
  rootly-cli war-room INC-1234 --agent
  ```
- **`fixed-last-time`**  -  Mine the resolution notes and action items from a service's past incidents to surface what actually resolved this class of problem.

  _Pick this when an alert smells like a repeat  -  it answers 'what did we do last time' from history instead of guesswork._

  ```bash
  rootly-cli fixed-last-time <service-or-query> --agent --limit 10
  ```
- **`postmortem-skeleton`**  -  Emit a paste-ready post-mortem markdown skeleton for an incident: timeline, action items, severity, duration, and affected services.

  _Use at the start of a retrospective to skip the copy-paste assembly and go straight to analysis._

  ```bash
  rootly-cli postmortem-skeleton INC-1234
  ```
- **`action-items-overdue`**  -  List every open or overdue incident action item across all incidents, grouped by owner or team, with incident, severity, and age.

  _Reach for this in the weekly review to see which post-incident commitments are slipping and who owns them._

  ```bash
  rootly-cli action-items-overdue --group-by owner --agent
  ```
- **`digest`**  -  Time-windowed rollup of everything that moved since a timestamp: incidents opened/resolved, severity changes, new action items, alerts fired.

  _Reach for this after time away or before standup for a portfolio-wide what-moved rollup; use handoff for shift-scoped summaries._

  ```bash
  rootly-cli digest --since 24h --agent
  ```

### On-call operations
- **`coverage-gaps`**  -  Scan every on-call schedule for unstaffed windows over the next N days so you catch holiday and weekend gaps before a page is missed.

  _Run this on Monday (or in CI) to find on-call holes proactively rather than after someone misses an alert._

  ```bash
  rootly-cli coverage-gaps --days 14 --agent
  ```
- **`oncall-now`**  -  Show who is on call right now across every schedule and service in one table, escalation tier included.

  _Use when a page fires and you need to know who is responsible across all services without opening each schedule._

  ```bash
  rootly-cli oncall-now --agent
  ```
- **`handoff`**  -  End-of-shift summary: incidents opened, closed, and still open during the outgoing shift window, plus open action items and severity mix.

  _Run at shift change so the next on-call inherits an accurate picture instead of a retyped-from-memory note._

  ```bash
  rootly-cli handoff --schedule primary-oncall --agent
  ```
- **`oncall-load`**  -  Rank people by on-call hours and pages received across every schedule, so uneven rotations and burnout risk surface before someone quits.

  _Reach for this when asked who is overloaded, whether the rotation is fair, or who absorbed the most pages this month._

  ```bash
  rootly-cli oncall-load --days 30 --agent
  ```
- **`escalation-trace`**  -  Print the full ordered escalation ladder for a service or incident  -  every rung, the policy it comes from, who currently sits at each level, and the delay before the next page.

  _Reach for this during a page to answer 'who is next if nobody acks' without walking four API objects by hand._

  ```bash
  rootly-cli escalation-trace --service checkout-api --agent
  ```

### Reliability analytics
- **`mttr`**  -  Compute mean time to acknowledge and resolve from incident timestamps, grouped by service, team, or severity, with optional volume counts.

  _Use for the weekly reliability review or to quantify whether a service is trending worse over time._

  ```bash
  rootly-cli mttr --by service --since 30d --agent
  ```
- **`service-health`**  -  Per-service scorecard: incident count, MTTR, last incident, open action items, current on-call, and SLA status in one row.

  _Pull this before a deploy or during a portfolio review to judge a service's health at a glance._

  ```bash
  rootly-cli service-health <service> --agent
  ```
- **`sla-breach`**  -  List incidents that have breached or are about to breach their SLA target, sorted by time remaining, with a non-zero exit when any active breach exists.

  _Reach for this to gate dashboards or escalations on real SLA risk; exit code makes it pipeline-wireable._

  ```bash
  rootly-cli sla-breach --within 2h --agent
  ```

### CI / deploy plumbing
- **`deploy-guard`**  -  Pre-deploy gate for a service: checks for an open incident, confirms someone is on call, and flags recent flakiness, returning a non-zero exit code when it is unsafe to ship.

  _Wire into a deploy script so a risky push is blocked when the target service is mid-incident or has no on-call coverage._

  ```bash
  rootly-cli deploy-guard <service> --within 7d
  ```

### Config & signal hygiene
- **`config-diff`**  -  Diff the current synced Rootly config (services, escalation policies, workflows, severities, schedules) against the last saved snapshot to see what was added, removed, or changed  -  'config-diff --save' records the baseline.

  _Reach for this when auditing whether live Rootly config drifted from what config-as-code thinks it manages._

  ```bash
  rootly-cli config-diff --agent
  ```
- **`alert-noise`**  -  Rank alert sources and services by alert volume, repeat-fire rate, and how many alerts never became incidents  -  the signal-to-noise view that finds a flapping integration.

  _Reach for this when on-call is drowning: it names the noisiest source and the alerts that never become real incidents._

  ```bash
  rootly-cli alert-noise --days 14 --agent
  ```

## Recipes


### Triage a familiar-looking incident

```bash
rootly-cli related INC-1234 --agent --limit 5
```

Surface the five most similar past incidents so you can reuse a known resolution path.

### Narrow a verbose incident list for an agent

```bash
rootly-cli incidents list --agent --select data.id,data.attributes.title,data.attributes.created_at,data.attributes.severity
```

JSON:API returns tens of KB per call; dotted --select returns only the fields an agent needs so it does not burn context parsing the rest.

### Find Monday's on-call holes

```bash
rootly-cli coverage-gaps --days 14 --agent
```

Report every unstaffed on-call window in the next two weeks across all schedules before a page is missed.

### Gate a deploy on service health

```bash
rootly-cli deploy-guard <service> --within 7d
```

Exit non-zero when checkout-api has an open incident, no on-call, or recent flakiness  -  drop it into a deploy script.

### Build the weekly reliability review

```bash
rootly-cli service-health --agent
```

Emit a per-service scorecard (incidents, MTTR, open action items, on-call, SLA) for the whole portfolio in one pass.

## Usage

Run `rootly-cli --help` for the full command reference and flag list.

## Commands

### action-items

Manage action items

- **`rootly-cli action-items delete-incident`** - Delete a specific incident action item by id
- **`rootly-cli action-items get-incident`** - Retrieves a specific incident_action_item by id
- **`rootly-cli action-items list-all-incident`** - List all action items for an organization
- **`rootly-cli action-items update-incident`** - Update a specific incident action item by id

### alert-events

Manage alert events

- **`rootly-cli alert-events delete`** - Deletes a specific alert event. Only alert events with kind 'note' (user-created notes) can be deleted. System-generated events are immutable to maintain audit trail integrity.
- **`rootly-cli alert-events get`** - Retrieves a specific alert_event by id
- **`rootly-cli alert-events update`** - Updates a specific alert event. Only alert events with kind 'note' (user-created notes) can be updated. System-generated events are immutable to maintain audit trail integrity.

### alert-fields

Manage alert fields

- **`rootly-cli alert-fields create`** - Creates a new alert field from provided data
- **`rootly-cli alert-fields delete`** - Delete a specific alert field by id
- **`rootly-cli alert-fields get`** - Retrieves a specific alert field by id
- **`rootly-cli alert-fields list`** - List alert fields
- **`rootly-cli alert-fields update`** - Update a specific alert field by id

### alert-groups

Manage alert groups

- **`rootly-cli alert-groups create`** - Creates a new alert group. **Note**: For enhanced functionality and future compatibility, consider using the advanced alert grouping with `conditions` field instead of the legacy `group_by_alert_title`, `group_by_alert_urgency`, and `attributes` fields.
- **`rootly-cli alert-groups delete`** - Delete a specific alert group by id
- **`rootly-cli alert-groups get`** - Retrieves a specific alert group by id
- **`rootly-cli alert-groups list`** - List alert groups
- **`rootly-cli alert-groups update`** - Update a specific alert group by id. **Note**: For enhanced functionality and future compatibility, consider using the advanced alert grouping with `conditions` field instead of the legacy `group_by_alert_title`, `group_by_alert_urgency`, and `attributes` fields.

### alert-routes

Manage alert routes

- **`rootly-cli alert-routes create`** - Creates a new alert route from provided data. **Note: This endpoint requires access to Advanced Alert Routing. If you're unsure whether you have access to this feature, please contact Rootly customer support.**

## Asynchronous Rule Creation

For organizations with large numbers of routing rules, Rootly supports asynchronous rule processing to improve performance. When enabled, rule creation happens in the background.

**Important**: When async processing is enabled, the rules list in the API response will not be up-to-date immediately after creation. You should refetch the alert route after a few minutes to get the updated rules.

If you experience slow operations when managing alert routes with many rules, contact Rootly customer support to enable asynchronous rule processing for your organization.
- **`rootly-cli alert-routes delete`** - Delete a specific alert route by id. **Note: This endpoint requires access to Advanced Alert Routing. If you're unsure whether you have access to this feature, please contact Rootly customer support.**
- **`rootly-cli alert-routes get`** - Get a specific alert route by id. **Note: This endpoint requires access to Advanced Alert Routing. If you're unsure whether you have access to this feature, please contact Rootly customer support.**

## Optional Parameters

- **show_nested_ids** (query parameter): When set to `true`, the response will include IDs for all nested resources (destinations, condition_groups, conditions). This is useful when you need to reference these nested resources for updates or deletions via PATCH requests.

Example: `GET /v1/alert_routes/{id}?show_nested_ids=true`
- **`rootly-cli alert-routes list`** - List all alert routes for the current team with filtering and pagination. **Note: This endpoint requires access to Advanced Alert Routing. If you're unsure whether you have access to this feature, please contact Rootly customer support.**
- **`rootly-cli alert-routes patch`** - Updates an alert route. **Note: This endpoint requires access to Advanced Alert Routing. If you're unsure whether you have access to this feature, please contact Rootly customer support.**
- **`rootly-cli alert-routes update`** - Update a specific alert route by id. **Note: This endpoint requires access to Advanced Alert Routing. If you're unsure whether you have access to this feature, please contact Rootly customer support.**

### Asynchronous Rule Creation

For organizations with large numbers of routing rules, Rootly supports asynchronous rule processing to improve performance. When enabled, rule updates happen in the background.

**Important**: When async processing is enabled, the rules list in the API response will not be up-to-date immediately after update. You should refetch the alert route after a few minutes to get the updated rules.

If you experience slow operations when managing alert routes with many rules, contact Rootly customer support to enable asynchronous rule processing for your organization.

### alert-routing-rules

Manage alert routing rules

- **`rootly-cli alert-routing-rules create`** - Creates a new alert routing rule from provided data. **Note: If you are an advanced alert routing user, you should use the Alert Routes endpoint instead of this endpoint. If you don't know whether you are an advanced user, please contact Rootly customer support.**
- **`rootly-cli alert-routing-rules delete`** - Delete a specific alert routing rule by id. **Note: If you are an advanced alert routing user, you should use the Alert Routes endpoint instead of this endpoint. If you don't know whether you are an advanced user, please contact Rootly customer support.**
- **`rootly-cli alert-routing-rules get`** - Retrieves a specific alert routing rule by id. **Note: If you are an advanced alert routing user, you should use the Alert Routes endpoint instead of this endpoint. If you don't know whether you are an advanced user, please contact Rootly customer support.**
- **`rootly-cli alert-routing-rules list`** - List alert routing rules. **Note: If you are an advanced alert routing user, you should use the Alert Routes endpoint instead of this endpoint. If you don't know whether you are an advanced user, please contact Rootly customer support.**
- **`rootly-cli alert-routing-rules update`** - Update a specific alert routing rule by id. **Note: If you are an advanced alert routing user, you should use the Alert Routes endpoint instead of this endpoint. If you don't know whether you are an advanced user, please contact Rootly customer support.**

### alert-sources

Manage alert sources

- **`rootly-cli alert-sources create-alerts-source`** - Creates a new alert source from provided data
- **`rootly-cli alert-sources delete-alerts-source`** - Delete a specific alert source by id
- **`rootly-cli alert-sources get-alerts-source`** - Retrieves a specific alert source by id
- **`rootly-cli alert-sources list-alerts-sources`** - List alert sources
- **`rootly-cli alert-sources update-alerts-source`** - Update a specific alert source by id

### alert-urgencies

Manage alert urgencies

- **`rootly-cli alert-urgencies create-alert-urgency`** - Creates a new alert urgency from provided data
- **`rootly-cli alert-urgencies delete-alert-urgency`** - Delete a specific alert urgency by id
- **`rootly-cli alert-urgencies get-alert-urgency`** - Retrieves a specific alert urgency by id
- **`rootly-cli alert-urgencies list`** - List alert urgencies
- **`rootly-cli alert-urgencies update-alert-urgency`** - Update a specific alert urgency by id

### alerts

Manage alerts

- **`rootly-cli alerts create`** - Creates a new alert from provided data
- **`rootly-cli alerts get`** - Retrieves a specific alert by id
- **`rootly-cli alerts list`** - List alerts
- **`rootly-cli alerts update`** - Updates an alert

### api-keys

Manage api keys

- **`rootly-cli api-keys create`** - Creates a new API key and returns it with the plaintext token. **The token is only returned once**  -  store it securely, as it cannot be retrieved again.

**Kinds and required fields:**
- `personal`  -  created for the authenticated user. No additional fields required.
- `team`  -  scoped to a team (group). Requires `group_id`. A service account is automatically created with permissions derived from group membership.
- `organization`  -  organization-wide access. Requires owner or admin role. Optionally set `role_id` and `on_call_role_id` to control the service account's permissions.

**Expiration:** All keys require an `expires_at` date set in the future (maximum 5 years). Names must be unique within their kind and scope.
- **`rootly-cli api-keys delete`** - Revoke an API key. The key is immediately invalidated and can no longer be used for authentication. This action cannot be undone.

For `team` and `organization` keys, the associated service account is also deleted. Any active sessions using this key will fail on the next request.
- **`rootly-cli api-keys get`** - Retrieves a specific API key by its UUID. Returns key metadata including name, kind, expiration, last usage timestamp, and the grace period status  -  the secret token is never included.
- **`rootly-cli api-keys list`** - List API keys for the current organization. Returns key metadata including name, kind, expiration, and last usage  -  the secret token value is never included in the response.

**API key kinds:**
- `personal`  -  tied to a specific user, inherits that user's permissions.
- `team`  -  scoped to one or more teams (groups), creates a service account with permissions derived from group membership.
- `organization`  -  organization-wide, creates a service account with a configurable role and on-call role.

**Automated rotation workflow:** Use `filter[expires_at][lt]` to find keys approaching expiration, then call the rotate endpoint to issue a new token before the old one expires. Combine with `filter[active]=true` to exclude already-expired keys.

**Sorting:** Use the `sort` parameter with a field name (e.g., `sort=expires_at`). Prefix with `-` for descending order (e.g., `sort=-created_at`). Allowed fields: `name`, `kind`, `created_at`, `updated_at`, `expires_at`, `last_used_at`.
- **`rootly-cli api-keys update`** - Update an API key's mutable attributes: `name`, `description`, and `expires_at`.

The key's `kind`, `role_id`, `on_call_role_id`, and token cannot be changed after creation. To issue a new token, use the rotate endpoint. To change the role or kind, revoke the key and create a new one.

The new `expires_at` must be in the future and within 5 years.

### audits

Manage audits

- **`rootly-cli audits`** - List audits

### authorizations

Manage authorizations

- **`rootly-cli authorizations create`** - Creates a new authorization from provided data
- **`rootly-cli authorizations delete`** - Delete a specific authorization by id
- **`rootly-cli authorizations get`** - Retrieves a specific authorization by id
- **`rootly-cli authorizations list`** - List authorizations
- **`rootly-cli authorizations update`** - Update a specific authorization by id

### catalog-checklist-templates

Manage catalog checklist templates

- **`rootly-cli catalog-checklist-templates create`** - Creates a new catalog checklist template
- **`rootly-cli catalog-checklist-templates delete`** - Delete a specific catalog checklist template by id
- **`rootly-cli catalog-checklist-templates get`** - Retrieves a specific catalog checklist template by id
- **`rootly-cli catalog-checklist-templates list`** - List catalog checklist templates
- **`rootly-cli catalog-checklist-templates update`** - Update a specific catalog checklist template by id

### catalog-entities

Manage catalog entities

- **`rootly-cli catalog-entities delete-catalog-entity`** - Delete a specific Catalog Entity by id
- **`rootly-cli catalog-entities get-catalog-entity`** - Retrieves a specific Catalog Entity by id
- **`rootly-cli catalog-entities update-catalog-entity`** - Update a specific Catalog Entity by id

### catalog-entity-checklists

Manage catalog entity checklists

- **`rootly-cli catalog-entity-checklists get`** - Retrieves a specific catalog entity checklist by id
- **`rootly-cli catalog-entity-checklists list`** - List catalog entity checklists

### catalog-entity-properties

Manage catalog entity properties

- **`rootly-cli catalog-entity-properties delete-catalog-entity-property`** - **Deprecated:** This endpoint is deprecated, please use the `fields` attribute on catalog entities or native catalog endpoints (teams, services, functionalities, incident_types, causes, environments) to set field values instead.

Delete a specific Catalog Entity Property by id.
- **`rootly-cli catalog-entity-properties get-catalog-entity-property`** - **Deprecated:** This endpoint is deprecated, please use `include=fields` on catalog entities or native catalog endpoints (teams, services, functionalities, incident_types, causes, environments) to retrieve field values instead.

Retrieves a specific Catalog Entity Property by id.
- **`rootly-cli catalog-entity-properties update-catalog-entity-property`** - **Deprecated:** This endpoint is deprecated, please use the `fields` attribute on catalog entities or native catalog endpoints (teams, services, functionalities, incident_types, causes, environments) to set field values instead.

Update a specific Catalog Entity Property by id.

### catalog-properties

Manage catalog properties

- **`rootly-cli catalog-properties delete-catalog-property`** - Delete a specific catalog_property by id - returns catalog_properties type
- **`rootly-cli catalog-properties get-catalog-property`** - Retrieves a specific Catalog Property by id - returns catalog_properties type
- **`rootly-cli catalog-properties update-catalog-property`** - Update a specific catalog_property by id - returns catalog_properties type

### catalogs

Manage catalogs

- **`rootly-cli catalogs create`** - Creates a new catalog from provided data
- **`rootly-cli catalogs delete`** - Delete a specific catalog by id
- **`rootly-cli catalogs get`** - Retrieves a specific catalog by id
- **`rootly-cli catalogs list`** - List catalogs
- **`rootly-cli catalogs update`** - Update a specific catalog by id

### causes

Manage causes

- **`rootly-cli causes create`** - Creates a new cause from provided data
- **`rootly-cli causes create-catalog-property`** - Creates a new Catalog Property from provided data
- **`rootly-cli causes delete`** - Delete a specific cause by id
- **`rootly-cli causes get`** - Retrieves a specific cause by id
- **`rootly-cli causes list`** - List causes
- **`rootly-cli causes list-catalog-properties`** - List Cause Catalog Properties
- **`rootly-cli causes update`** - Update a specific cause by id

### communications

Manage communications

- **`rootly-cli communications create-group`** - Creates a new communications group from provided data
- **`rootly-cli communications create-stage`** - Creates a new communications stage from provided data
- **`rootly-cli communications create-template`** - Creates a new communications template from provided data
- **`rootly-cli communications create-type`** - Creates a new communications type from provided data
- **`rootly-cli communications delete-group`** - Deletes a communications group
- **`rootly-cli communications delete-stage`** - Deletes a communications stage
- **`rootly-cli communications delete-template`** - Deletes a communications template
- **`rootly-cli communications delete-type`** - Deletes a communications type
- **`rootly-cli communications get-group`** - Shows details of a communications group
- **`rootly-cli communications get-stage`** - Shows details of a communications stage
- **`rootly-cli communications get-template`** - Shows details of a communications template
- **`rootly-cli communications get-type`** - Shows details of a communications type
- **`rootly-cli communications list-groups`** - Lists communications groups
- **`rootly-cli communications list-stages`** - Lists communications stages
- **`rootly-cli communications list-templates`** - Lists communications templates
- **`rootly-cli communications list-types`** - Lists communications types
- **`rootly-cli communications update-group`** - Updates a communications group
- **`rootly-cli communications update-stage`** - Updates a communications stage
- **`rootly-cli communications update-template`** - Updates a communications template
- **`rootly-cli communications update-type`** - Updates a communications type

### custom-field-options

Manage custom field options

- **`rootly-cli custom-field-options delete`** - [DEPRECATED] Use form field endpoints instead. Delete a specific Custom Field Option by id
- **`rootly-cli custom-field-options get`** - [DEPRECATED] Use form field endpoints instead. Retrieves a specific custom field option by id
- **`rootly-cli custom-field-options update`** - [DEPRECATED] Use form field endpoints instead. Update a specific custom field option by id

### custom-fields

Manage custom fields

- **`rootly-cli custom-fields create`** - [DEPRECATED] Use form field endpoints instead. Creates a new custom field from provided data
- **`rootly-cli custom-fields delete`** - [DEPRECATED] Use form field endpoints instead. Delete a specific custom field by id
- **`rootly-cli custom-fields get`** - Retrieves a specific custom_field by id
- **`rootly-cli custom-fields list`** - [DEPRECATED] Use form field endpoints instead. List Custom fields
- **`rootly-cli custom-fields update`** - [DEPRECATED] Use form field endpoints instead. Update a specific custom field by id

### custom-forms

Manage custom forms

- **`rootly-cli custom-forms create`** - Creates a new custom form from provided data
- **`rootly-cli custom-forms delete`** - Delete a specific custom form by id
- **`rootly-cli custom-forms get`** - Retrieves a specific custom form by id
- **`rootly-cli custom-forms list`** - List custom forms
- **`rootly-cli custom-forms update`** - Update a specific custom form by id

### dashboard-panels

Manage dashboard panels

- **`rootly-cli dashboard-panels delete`** - Delete a specific dashboard panel by id
- **`rootly-cli dashboard-panels get`** - Retrieves a specific dashboard panel by id
- **`rootly-cli dashboard-panels update`** - Update a specific dashboard panel by id

### dashboards

Manage dashboards

- **`rootly-cli dashboards create`** - Creates a new dashboard from provided data
- **`rootly-cli dashboards delete`** - Delete a specific dashboard by id
- **`rootly-cli dashboards get`** - Retrieves a specific dashboard by id
- **`rootly-cli dashboards list`** - List dashboards
- **`rootly-cli dashboards update`** - Update a specific dashboard by id

### edge-connectors

Manage edge connectors

- **`rootly-cli edge-connectors create`** - Create edge connector
- **`rootly-cli edge-connectors delete`** - Delete edge connector
- **`rootly-cli edge-connectors get`** - Show edge connector
- **`rootly-cli edge-connectors list`** - List edge connectors
- **`rootly-cli edge-connectors update`** - Update edge connector

### email-addresses

Manage email addresses

- **`rootly-cli email-addresses delete-user-email-address`** - Deletes a user email address
- **`rootly-cli email-addresses show-user-email-address`** - Retrieves a specific user email address
- **`rootly-cli email-addresses update-user-email-address`** - Updates a user email address

### environments

Manage environments

- **`rootly-cli environments create`** - Creates a new environment from provided data
- **`rootly-cli environments create-catalog-property`** - Creates a new Catalog Property from provided data
- **`rootly-cli environments delete`** - Delete a specific environment by id
- **`rootly-cli environments get`** - Retrieves a specific environment by id
- **`rootly-cli environments list`** - List environments
- **`rootly-cli environments list-catalog-properties`** - List Environment Catalog Properties
- **`rootly-cli environments update`** - Update a specific environment by id

### escalation-levels

Manage escalation levels

- **`rootly-cli escalation-levels delete`** - Delete a specific escalation level by id
- **`rootly-cli escalation-levels get`** - Retrieves a specific escalation level by id
- **`rootly-cli escalation-levels update`** - Update a specific escalation level by id

### escalation-paths

Manage escalation paths

- **`rootly-cli escalation-paths delete`** - Delete a specific escalation path by id
- **`rootly-cli escalation-paths get`** - Retrieves a specific escalation path by id
- **`rootly-cli escalation-paths update`** - Update a specific escalation path by id

### escalation-policies

Manage escalation policies

- **`rootly-cli escalation-policies create-escalation-policy`** - Creates a new escalation policy from provided data
- **`rootly-cli escalation-policies delete-escalation-policy`** - Delete a specific escalation policy by id
- **`rootly-cli escalation-policies get-escalation-policy`** - Retrieves a specific escalation policy by id
- **`rootly-cli escalation-policies list`** - List escalation policies
- **`rootly-cli escalation-policies update-escalation-policy`** - Update a specific escalation policy by id

### events

Manage events

- **`rootly-cli events delete-incident`** - Delete a specific incident event by id
- **`rootly-cli events get-incident`** - Retrieves a specific incident_event by id
- **`rootly-cli events update-incident`** - Update a specific incident event by id

### feedbacks

Manage feedbacks

- **`rootly-cli feedbacks get-incident`** - Retrieves a specific incident_feedback by id
- **`rootly-cli feedbacks update-incident`** - Update a specific incident feedback by id

### form-field-options

Manage form field options

- **`rootly-cli form-field-options delete`** - Delete a specific form_field_option by id
- **`rootly-cli form-field-options get`** - Retrieves a specific form_field_option by id
- **`rootly-cli form-field-options update`** - Update a specific form_field_option by id

### form-field-placement-conditions

Manage form field placement conditions

- **`rootly-cli form-field-placement-conditions delete`** - Delete a specific form_field_placement_condition by id
- **`rootly-cli form-field-placement-conditions get`** - Retrieves a specific form_field_placement_condition by id
- **`rootly-cli form-field-placement-conditions update`** - Update a specific form_field_placement_condition by id

### form-field-placements

Manage form field placements

- **`rootly-cli form-field-placements delete`** - Delete a specific form_field_placement by id
- **`rootly-cli form-field-placements get`** - Retrieves a specific form_field_placement by id
- **`rootly-cli form-field-placements update`** - Update a specific form_field_placement by id

### form-field-positions

Manage form field positions

- **`rootly-cli form-field-positions delete`** - Delete a specific form_field position by id
- **`rootly-cli form-field-positions get`** - Retrieves a specific form field_position by id
- **`rootly-cli form-field-positions update`** - Update a specific form_field position by id

### form-fields

Manage form fields

- **`rootly-cli form-fields create`** - Creates a new form_field from provided data
- **`rootly-cli form-fields delete`** - Delete a specific form_field by id
- **`rootly-cli form-fields get`** - Retrieves a specific form_field by id
- **`rootly-cli form-fields list`** - List form_fields
- **`rootly-cli form-fields update`** - Update a specific form_field by id

### form-set-conditions

Manage form set conditions

- **`rootly-cli form-set-conditions delete`** - Delete a specific form_set_condition by id
- **`rootly-cli form-set-conditions get`** - Retrieves a specific form_set_condition by id
- **`rootly-cli form-set-conditions update`** - Update a specific form_set_condition by id

### form-sets

Manage form sets

- **`rootly-cli form-sets create`** - Creates a new form_set from provided data
- **`rootly-cli form-sets delete`** - Delete a specific form_set by id
- **`rootly-cli form-sets get`** - Retrieves a specific form_set by id
- **`rootly-cli form-sets list`** - List form_sets
- **`rootly-cli form-sets update`** - Update a specific form_set by id

### functionalities

Manage functionalities

- **`rootly-cli functionalities create-functionality`** - Creates a new functionality from provided data
- **`rootly-cli functionalities create-functionality-catalog-property`** - Creates a new Catalog Property from provided data
- **`rootly-cli functionalities delete-functionality`** - Delete a specific functionality by id
- **`rootly-cli functionalities get-functionality`** - Retrieves a specific functionality by id
- **`rootly-cli functionalities list`** - List functionalities
- **`rootly-cli functionalities list-functionality-catalog-properties`** - List Functionality Catalog Properties
- **`rootly-cli functionalities update-functionality`** - Update a specific functionality by id

### heartbeats

Manage heartbeats

- **`rootly-cli heartbeats create`** - Creates a new heartbeat from provided data
- **`rootly-cli heartbeats delete`** - Delete a specific heartbeat by id
- **`rootly-cli heartbeats get`** - Retrieves a specific heartbeat by id
- **`rootly-cli heartbeats list`** - List heartbeats
- **`rootly-cli heartbeats update`** - Update a specific heartbeat by id

### incident-custom-field-selections

Manage incident custom field selections

- **`rootly-cli incident-custom-field-selections delete`** - [DEPRECATED] Use form field endpoints instead. Delete a specific incident custom field selection by id
- **`rootly-cli incident-custom-field-selections get`** - [DEPRECATED] Use form field endpoints instead. Retrieves a specific incident custom field selection by id
- **`rootly-cli incident-custom-field-selections update`** - [DEPRECATED] Use form field endpoints instead. Update a specific incident custom field selection by id

### incident-event-functionalities

Manage incident event functionalities

- **`rootly-cli incident-event-functionalities delete-incident-event-functionality`** - Delete a specific incident event functionality by id
- **`rootly-cli incident-event-functionalities get`** - Retrieves a specific incident_event_functionality by id
- **`rootly-cli incident-event-functionalities update-incident-event-functionality`** - Update a specific incident event functionality by id

### incident-event-services

Manage incident event services

- **`rootly-cli incident-event-services delete`** - Delete a specific incident event service by id
- **`rootly-cli incident-event-services get`** - Retrieves a specific incident_event_service by id
- **`rootly-cli incident-event-services update`** - Update a specific incident event service by id

### incident-form-field-selections

Manage incident form field selections

- **`rootly-cli incident-form-field-selections delete`** - Delete a specific incident form field selection by id
- **`rootly-cli incident-form-field-selections get`** - Retrieves a specific incident form field selection by id
- **`rootly-cli incident-form-field-selections update`** - Update a specific incident form field selection by id

### incident-permission-set-booleans

Manage incident permission set booleans

- **`rootly-cli incident-permission-set-booleans delete`** - Delete a specific incident_permission_set_boolean by id
- **`rootly-cli incident-permission-set-booleans get`** - Retrieves a specific incident_permission_set_boolean by id
- **`rootly-cli incident-permission-set-booleans update`** - Update a specific incident_permission_set_boolean by id

### incident-permission-set-resources

Manage incident permission set resources

- **`rootly-cli incident-permission-set-resources delete`** - Delete a specific incident_permission_set_resource by id
- **`rootly-cli incident-permission-set-resources get`** - Retrieves a specific incident_permission_set_resource by id
- **`rootly-cli incident-permission-set-resources update`** - Update a specific incident_permission_set_resource by id

### incident-permission-sets

Manage incident permission sets

- **`rootly-cli incident-permission-sets create`** - Creates a new incident_permission_set from provided data
- **`rootly-cli incident-permission-sets delete`** - Delete a specific incident_permission_set by id
- **`rootly-cli incident-permission-sets get`** - Retrieves a specific incident_permission_set by id
- **`rootly-cli incident-permission-sets list`** - List incident_permission_sets
- **`rootly-cli incident-permission-sets update`** - Update a specific incident_permission_set by id

### incident-retrospective-steps

Manage incident retrospective steps

- **`rootly-cli incident-retrospective-steps get`** - Retrieves a specific incident retrospective step by id
- **`rootly-cli incident-retrospective-steps update`** - Update a specific incident retrospective step by id

### incident-role-tasks

Manage incident role tasks

- **`rootly-cli incident-role-tasks delete`** - Delete a specific incident_role task by id
- **`rootly-cli incident-role-tasks get`** - Retrieves a specific incident_role_task by id
- **`rootly-cli incident-role-tasks update`** - Update a specific incident_role task by id

### incident-roles

Manage incident roles

- **`rootly-cli incident-roles create`** - Creates a new incident role from provided data
- **`rootly-cli incident-roles delete`** - Delete a specific incident_role by id
- **`rootly-cli incident-roles get`** - Retrieves a specific incident_role by id
- **`rootly-cli incident-roles list`** - List incident roles
- **`rootly-cli incident-roles update`** - Update a specific incident_role by id

### incident-sub-statuses

Manage incident sub statuses

- **`rootly-cli incident-sub-statuses delete-incident-sub-status`** - Delete a specific incident_sub_status by id
- **`rootly-cli incident-sub-statuses get-incident-sub-status`** - Retrieves a specific incident_sub_status by id
- **`rootly-cli incident-sub-statuses update-incident-sub-status`** - Update a specific incident_sub_status by id

### incident-types

Manage incident types

- **`rootly-cli incident-types create`** - Creates a new incident_type from provided data
- **`rootly-cli incident-types create-catalog-property`** - Creates a new Catalog Property from provided data
- **`rootly-cli incident-types delete`** - Delete a specific incident_type by id
- **`rootly-cli incident-types get`** - Retrieves a specific incident_type by id
- **`rootly-cli incident-types list`** - List incident types
- **`rootly-cli incident-types list-catalog-properties`** - List IncidentType Catalog Properties
- **`rootly-cli incident-types update`** - Update a specific incident_type by id

### incidents

Manage incidents

- **`rootly-cli incidents create`** - Creates a new incident from provided data
- **`rootly-cli incidents delete`** - Delete a specific incident by id
- **`rootly-cli incidents get`** - Retrieves a specific incident by id
- **`rootly-cli incidents list`** - List incidents
- **`rootly-cli incidents update`** - Update a specific incident by id

### ip-ranges

Manage ip ranges

- **`rootly-cli ip-ranges`** - Retrieves the IP ranges for rootly.com services

### live-call-routers

Manage live call routers

- **`rootly-cli live-call-routers create`** - Creates a new Live Call Router from provided data
- **`rootly-cli live-call-routers delete`** - Delete a specific Live Call Router by id
- **`rootly-cli live-call-routers generate-phone-number`** - Generates a phone number for Live Call Router
- **`rootly-cli live-call-routers get`** - Retrieves a specific Live Call Router by id
- **`rootly-cli live-call-routers list`** - List Live Call Routers
- **`rootly-cli live-call-routers update`** - Update a specific Live Call Router by id

### meeting-recordings

Manage meeting recordings

- **`rootly-cli meeting-recordings delete`** - Delete a meeting recording. Only completed or failed recordings can be deleted. Active recordings (pending, recording, paused) must be stopped first.
- **`rootly-cli meeting-recordings get`** - Retrieve a single meeting recording session including its status, duration, speaker count, word count, and transcript summary.

### notification-rules

Manage notification rules

- **`rootly-cli notification-rules delete-user`** - Delete a specific user notification rule by id
- **`rootly-cli notification-rules get-user`** - Retrieves a specific user notification rule by id
- **`rootly-cli notification-rules update-user`** - Update a specific user notification rule by id

### on-call-pay-reports

Manage on call pay reports

- **`rootly-cli on-call-pay-reports create`** - Generates a new on-call pay report for the given date range. The report is generated asynchronously.
- **`rootly-cli on-call-pay-reports get`** - Retrieves a specific on-call pay report by id
- **`rootly-cli on-call-pay-reports list`** - List on-call pay reports
- **`rootly-cli on-call-pay-reports update`** - Update a specific on-call pay report by id. Triggers report regeneration.

### on-call-roles

Manage on call roles

- **`rootly-cli on-call-roles create`** - Creates a new On-Call Role from provided data
- **`rootly-cli on-call-roles delete`** - Delete a specific On-Call Role by id
- **`rootly-cli on-call-roles get`** - Retrieves a specific On-Call Role by id
- **`rootly-cli on-call-roles list`** - List On-Call Roles
- **`rootly-cli on-call-roles update`** - Update a specific On-Call Role by id

### on-call-shadows

Manage on call shadows

- **`rootly-cli on-call-shadows delete`** - Delete a specific on call shadow configuration by id. Future shadows are hard-deleted. Active shadows (started in the past) have their end time truncated to preserve historical data.
- **`rootly-cli on-call-shadows get`** - Retrieves a specific On Call Shadow configuration by ID
- **`rootly-cli on-call-shadows update`** - Update a specific on call shadow configuration by id

### oncalls

Manage oncalls

- **`rootly-cli oncalls`** - List who is currently on-call, with support for filtering by escalation policy, schedule, and user. Returns on-call entries grouped by escalation policy level.

### override-shifts

Manage override shifts

- **`rootly-cli override-shifts delete`** - Delete a specific override shift by id
- **`rootly-cli override-shifts get`** - Retrieves a specific override shift by id
- **`rootly-cli override-shifts update`** - Update a specific override shift by id

### phone-numbers

Manage phone numbers

- **`rootly-cli phone-numbers delete-user`** - Deletes a user phone number
- **`rootly-cli phone-numbers show-user`** - Retrieves a specific user phone number
- **`rootly-cli phone-numbers update-user`** - Updates a user phone number

### playbook-tasks

Manage playbook tasks

- **`rootly-cli playbook-tasks delete`** - Delete a specific playbook task by id
- **`rootly-cli playbook-tasks get`** - Retrieves a specific playbook_task by id
- **`rootly-cli playbook-tasks update`** - Update a specific playbook task by id

### playbooks

Manage playbooks

- **`rootly-cli playbooks create`** - Creates a new playbook from provided data
- **`rootly-cli playbooks delete`** - Delete a specific playbook by id
- **`rootly-cli playbooks get`** - Retrieves a specific playbook by id
- **`rootly-cli playbooks list`** - List playbooks
- **`rootly-cli playbooks update`** - Update a specific playbook by id

### post-mortem-templates

Manage post mortem templates

- **`rootly-cli post-mortem-templates create-postmortem-template`** - Creates a new Retrospective Template from provided data
- **`rootly-cli post-mortem-templates delete-postmortem-template`** - Delete a specific Retrospective Template by id
- **`rootly-cli post-mortem-templates get-postmortem-template`** - Retrieves a specific Retrospective Template by id
- **`rootly-cli post-mortem-templates list-postmortem-templates`** - List Retrospective Templates
- **`rootly-cli post-mortem-templates update-postmortem-template`** - Update a specific Retrospective Template by id

### post-mortems

Manage post mortems

- **`rootly-cli post-mortems list-incident`** - List incident retrospectives
- **`rootly-cli post-mortems list-incident-postmortems`** - Retrieves an incident retrospective
- **`rootly-cli post-mortems update-incident`** - Update a specific incident retrospective by id

### pulses

Manage pulses

- **`rootly-cli pulses create`** - Creates a new pulse from provided data
- **`rootly-cli pulses get`** - Retrieves a specific pulse by id
- **`rootly-cli pulses list`** - List pulses
- **`rootly-cli pulses update`** - Update a specific pulse by id

### retrospective-configurations

Manage retrospective configurations

- **`rootly-cli retrospective-configurations get`** - Retrieves a specific retrospective_configuration by id
- **`rootly-cli retrospective-configurations list`** - List retrospective configurations
- **`rootly-cli retrospective-configurations update`** - Update a specific retrospective configuration by id

### retrospective-process-group-steps

Manage retrospective process group steps

- **`rootly-cli retrospective-process-group-steps delete`** - Delete a specific RetrospectiveProcessGroup Step by id
- **`rootly-cli retrospective-process-group-steps get`** - Retrieves a specific RetrospectiveProcessGroup Step by id
- **`rootly-cli retrospective-process-group-steps update`** - Update a specific RetrospectiveProcessGroup Step by id

### retrospective-process-groups

Manage retrospective process groups

- **`rootly-cli retrospective-process-groups delete`** - Delete a specific Retrospective Process Group by id
- **`rootly-cli retrospective-process-groups get`** - Retrieves a specific Retrospective Process Group by id
- **`rootly-cli retrospective-process-groups update`** - Update a specific Retrospective Process Group by id

### retrospective-processes

Manage retrospective processes

- **`rootly-cli retrospective-processes create-retrospective-process`** - Creates a new retrospective process from provided data
- **`rootly-cli retrospective-processes delete-retrospective-process`** - Delete a specific retrospective process by id
- **`rootly-cli retrospective-processes get-retrospective-process`** - Retrieves a specific retrospective process by id
- **`rootly-cli retrospective-processes list`** - List retrospective processes
- **`rootly-cli retrospective-processes update-retrospective-process`** - Updates a specific retrospective process by id

### retrospective-steps

Manage retrospective steps

- **`rootly-cli retrospective-steps delete`** - Delete a specific retrospective step by id
- **`rootly-cli retrospective-steps get`** - Retrieves a specific retrospective step by id
- **`rootly-cli retrospective-steps update`** - Update a specific retrospective step by id

### roles

Manage roles

- **`rootly-cli roles create`** - Creates a new role from provided data
- **`rootly-cli roles delete`** - Delete a specific role by id
- **`rootly-cli roles get`** - Retrieves a specific role by id
- **`rootly-cli roles list`** - List roles
- **`rootly-cli roles update`** - Update a specific role by id

### schedule-rotation-active-days

Manage schedule rotation active days

- **`rootly-cli schedule-rotation-active-days delete`** - Delete a specific schedule rotation active day
- **`rootly-cli schedule-rotation-active-days get`** - Retrieves a specific schedule rotation active day by id
- **`rootly-cli schedule-rotation-active-days update`** - Update a specific schedule rotation active day by id

### schedule-rotation-users

Manage schedule rotation users

- **`rootly-cli schedule-rotation-users delete`** - Delete a specific schedule rotation user by id
- **`rootly-cli schedule-rotation-users get`** - Retrieves a specific schedule rotation user by id
- **`rootly-cli schedule-rotation-users update`** - Update a specific schedule rotation user by id

### schedule-rotations

Manage schedule rotations

- **`rootly-cli schedule-rotations delete`** - Delete a specific schedule rotation by id
- **`rootly-cli schedule-rotations get`** - Retrieves a specific schedule rotation by id
- **`rootly-cli schedule-rotations update`** - Update a specific schedule rotation by id

### schedules

Manage schedules

- **`rootly-cli schedules create`** - Creates a new schedule from provided data
- **`rootly-cli schedules delete`** - Delete a specific schedule by id
- **`rootly-cli schedules get`** - Retrieves a specific schedule by id
- **`rootly-cli schedules list`** - List schedules
- **`rootly-cli schedules update`** - Updates a specific schedule by id

### secrets

Manage secrets

- **`rootly-cli secrets create`** - Creates a new secret from provided data
- **`rootly-cli secrets delete`** - Delete a specific secret by id
- **`rootly-cli secrets get`** - Retrieve a specific secret by id
- **`rootly-cli secrets list`** - List secrets
- **`rootly-cli secrets update`** - Update a specific secret by id

### services

Manage services

- **`rootly-cli services create`** - Creates a new service from provided data
- **`rootly-cli services create-catalog-property`** - Creates a new Catalog Property from provided data
- **`rootly-cli services delete`** - Delete a specific service by id
- **`rootly-cli services get`** - Retrieves a specific service by id
- **`rootly-cli services list`** - List services
- **`rootly-cli services list-catalog-properties`** - List Service Catalog Properties
- **`rootly-cli services update`** - Update a specific service by id

### severities

Manage severities

- **`rootly-cli severities create-severity`** - Creates a new severity from provided data
- **`rootly-cli severities delete-severity`** - Delete a specific severity by id
- **`rootly-cli severities get-severity`** - Retrieves a specific severity by id
- **`rootly-cli severities list`** - List severities
- **`rootly-cli severities update-severity`** - Update a specific severity by id

### shifts

Manage shifts

- **`rootly-cli shifts`** - List shifts

### slas

Manage slas

- **`rootly-cli slas create`** - Creates a new SLA from provided data
- **`rootly-cli slas delete`** - Delete a specific SLA by id
- **`rootly-cli slas get`** - Retrieves a specific SLA by id
- **`rootly-cli slas list`** - List SLAs
- **`rootly-cli slas update`** - Update a specific SLA by id

### status-page-events

Manage status page events

- **`rootly-cli status-page-events delete-incident-status-page`** - Delete a specific incident status page event by id
- **`rootly-cli status-page-events get-incident-status-pages`** - Retrieves a specific incident_status_page_event by id
- **`rootly-cli status-page-events update-incident-status-page`** - Update a specific incident status page event by id

### status-pages

Manage status pages

- **`rootly-cli status-pages create`** - Creates a new status page from provided data
- **`rootly-cli status-pages delete`** - Delete a specific status page by id
- **`rootly-cli status-pages get`** - Retrieves a specific status page by id
- **`rootly-cli status-pages list`** - List status pages
- **`rootly-cli status-pages update`** - Update a specific status page by id

### statuses

Manage statuses

- **`rootly-cli statuses get-status`** - Retrieves a specific Status by id
- **`rootly-cli statuses list`** - List Statuses

### sub-statuses

Manage sub statuses

- **`rootly-cli sub-statuses create-sub-status`** - Creates a new Sub-Status from provided data
- **`rootly-cli sub-statuses delete-sub-status`** - Delete a specific Sub-Status by id
- **`rootly-cli sub-statuses get-sub-status`** - Retrieves a specific Sub-Status by id
- **`rootly-cli sub-statuses list`** - List Sub-Statuses
- **`rootly-cli sub-statuses update-sub-status`** - Update a specific Sub-Status by id

### teams

Manage teams

- **`rootly-cli teams create`** - Creates a new team from provided data
- **`rootly-cli teams create-group-catalog-property`** - Creates a new Catalog Property from provided data
- **`rootly-cli teams delete`** - Delete a specific team by id
- **`rootly-cli teams get`** - Retrieves a specific team by id
- **`rootly-cli teams list`** - List teams
- **`rootly-cli teams list-group-catalog-properties`** - List Group Catalog Properties
- **`rootly-cli teams update`** - Update a specific team by id

### templates

Manage templates

- **`rootly-cli templates delete-status-page`** - Delete a specific template event by id
- **`rootly-cli templates get-status-page`** - Retrieves a specific status_page_template by id
- **`rootly-cli templates update-status-page`** - Update a specific template event by id

### users

Manage users

- **`rootly-cli users delete`** - Delete a specific user by id
- **`rootly-cli users get`** - Retrieves a specific user by id
- **`rootly-cli users get-current`** - Get current user
- **`rootly-cli users list`** - List users
- **`rootly-cli users update`** - Update a specific user by id

### webhooks

Manage webhooks

- **`rootly-cli webhooks create-endpoint`** - Creates a new webhook endpoint from provided data
- **`rootly-cli webhooks delete-endpoint`** - Delete a specific webhook endpoint by id
- **`rootly-cli webhooks deliver-delivery`** - Retries a webhook delivery
- **`rootly-cli webhooks get-delivery`** - Retrieves a specific webhook delivery by id
- **`rootly-cli webhooks get-endpoint`** - Retrieves a specific webhook endpoint by id
- **`rootly-cli webhooks list-deliveries`** - List webhook deliveries for given endpoint
- **`rootly-cli webhooks list-endpoints`** - List webhook endpoints
- **`rootly-cli webhooks update-endpoint`** - Update a specific webhook endpoint by id

### workflow-custom-field-selections

Manage workflow custom field selections

- **`rootly-cli workflow-custom-field-selections delete`** - [DEPRECATED] Use form field endpoints instead. Delete a specific workflow custom field selection by id
- **`rootly-cli workflow-custom-field-selections get`** - [DEPRECATED] Use form field endpoints instead. Retrieves a specific workflow custom field selection by id
- **`rootly-cli workflow-custom-field-selections update`** - [DEPRECATED] Use form field endpoints instead. Update a specific workflow custom field selection by id

### workflow-form-field-conditions

Manage workflow form field conditions

- **`rootly-cli workflow-form-field-conditions delete`** - Delete a specific workflow form field condition by id
- **`rootly-cli workflow-form-field-conditions get`** - Retrieves a specific workflow form field condition by id
- **`rootly-cli workflow-form-field-conditions update`** - Update a specific workflow form field condition by id

### workflow-groups

Manage workflow groups

- **`rootly-cli workflow-groups create`** - Creates a new workflow group from provided data
- **`rootly-cli workflow-groups delete`** - Delete a specific workflow group by id
- **`rootly-cli workflow-groups get`** - Retrieves a specific workflow group by id
- **`rootly-cli workflow-groups list`** - List workflow groups
- **`rootly-cli workflow-groups update`** - Update a specific workflow group by id

### workflow-tasks

Manage workflow tasks

- **`rootly-cli workflow-tasks delete`** - Delete a specific workflow task by id
- **`rootly-cli workflow-tasks get`** - Retrieves a specific workflow_task by id
- **`rootly-cli workflow-tasks update`** - Update a specific workflow task by id

### workflows

Manage workflows

- **`rootly-cli workflows create`** - Creates a new workflow from provided data
- **`rootly-cli workflows delete`** - Delete a specific workflow by id
- **`rootly-cli workflows get`** - Retrieves a specific workflow by id
- **`rootly-cli workflows list`** - List workflows
- **`rootly-cli workflows update`** - Update a specific workflow by id


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
rootly-cli alert-events get <id>

# JSON for scripting and agents
rootly-cli alert-events get <id> --json

# Filter to specific fields
rootly-cli alert-events get <id> --json --select id,name,status

# Dry run  -  show the request without sending
rootly-cli alert-events get <id> --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
rootly-cli alert-events get <id> --agent
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
rootly-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/rootly-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `ROOTLY_API_KEY` | per_call | Yes | Set to your API credential. |
| `ROOTLY_API_TOKEN` | per_call | No | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `rootly-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `rootly-cli doctor` to check credentials
- Verify the environment variable is set: `echo $ROOTLY_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call**  -  Set a valid key: export ROOTLY_API_KEY=<your-key> (from Rootly > Settings > API Keys), then run `rootly-cli doctor`.
- **429 Too Many Requests during a large sync**  -  The client honors Retry-After and backs off automatically; re-run `sync --full`  -  it resumes from the JSON:API page cursor.
- **related / mttr / coverage-gaps return empty**  -  These read the local mirror  -  run `rootly-cli sync --full` first so there is data to analyze.
- **A request fails with a media-type error**  -  Rootly is JSON:API; the CLI sends application/vnd.api+json automatically  -  upgrade if you hand-built a request body, and prefer the typed subcommands.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**rootly-cli**](https://github.com/rootlyhq/rootly-cli)  -  Go
- [**Rootly-MCP-server**](https://github.com/Rootly-AI-Labs/Rootly-MCP-server)  -  Python
- [**terraform-provider-rootly**](https://github.com/rootlyhq/terraform-provider-rootly)  -  Go
- [**rootly-go**](https://github.com/rootlyhq/rootly-go)  -  Go
- [**rootly-python**](https://github.com/rootlyhq/rootly-python)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
