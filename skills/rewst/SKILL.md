---
name: rewst
description: "Use when the user asks to check Rewst automation health, find failed or dormant workflows, report automation ROI/time-saved, compare config drift between client orgs, or check integration-pack coverage across tenants. Turns Rewst's GraphQL-only gateway into typed commands and adds cross-org rollups the web app makes you assemble one client at a time. Trigger phrases: `check rewst automation health`, `rewst failed workflows`, `how much time did rewst save`, `rewst config drift between orgs`, `which rewst orgs are missing a pack`, `use rewst`, `run rewst`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Rewst"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - rewst-cli
---

# Rewst Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `rewst-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/rewst/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/rewst/install.ps1 | iex
   ```
3. Verify: `rewst-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed with skill commands until verification succeeds.

Rewst's only surface is a GraphQL gateway with no first-party CLI and no local state. This CLI mirrors organizations, workflows, executions, variables, packs, triggers, forms, and the rest as typed commands with --json/--select/--agent output, an `api` command for full schema coverage, and offline SQLite sync. On top of that it adds multi-org rollups  -  fleet execution health, failure triage, automation ROI, dormant-workflow detection, cross-org config drift, and pack coverage  -  that an MSP operator otherwise has to assemble by clicking through the web app one client at a time.

## When to Use This CLI

Use this CLI to operate and monitor Rewst from the terminal or an agent: check whether automation is healthy for a client, triage failed runs, report how much time automation saved, find dormant workflows, compare configuration between tenants, and read or run any Rewst entity. It is the right tool when you want scriptable, JSON-first access to Rewst across many client organizations rather than clicking through the web console one org at a time.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI to build or visually edit workflow graphs  -  author those in the Rewst workflow designer.
- Do not use it as a long-running event listener or webhook receiver; for inbound triggers configure a Rewst trigger or the webhook URL instead.
- Do not use it to administer billing or the Rewst account itself; that lives in the Rewst console.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Fleet monitoring that compounds
- **`health`**  -  One-call workflow execution health for an org: succeeded / failed / running / pending counts plus time saved, with a clear unhealthy verdict when failures are present.

  _Reach for this first to answer 'is automation healthy for this client right now' without paging through the web app org by org._

  ```bash
  rewst-cli health --org 11111111-1111-1111-1111-111111111111 --since 24h --agent
  ```
- **`failures`**  -  Recent failed workflow executions for an org, newest first, with workflow id, status, task counts, and age  -  the triage queue the API has no single endpoint for.

  _Use this when something broke overnight and you need the failed runs to investigate, not every run._

  ```bash
  rewst-cli failures --org 11111111-1111-1111-1111-111111111111 --since 12h --limit 20 --agent
  ```
- **`dormant`**  -  Workflows that have not executed in N days  -  dead or orphaned automation that is still installed but no longer running.

  _Use this to clean up or re-enable automation that quietly went idle after a trigger or integration broke._

  ```bash
  rewst-cli dormant --org 11111111-1111-1111-1111-111111111111 --days 30 --agent
  ```

### ROI and reporting
- **`roi`**  -  Aggregates Rewst's humanSecondsSaved metric across an org's workflows into total hours/days saved and the top time-saving automations.

  _Reach for this to turn automation into a dollar/time story for a QBR or client report._

  ```bash
  rewst-cli roi --org 11111111-1111-1111-1111-111111111111 --since 30d --agent
  ```

### Multi-tenant governance
- **`drift`**  -  Compares org variables and installed packs between two organizations and reports what one has that the other is missing.

  _Reach for this when an automation works in one tenant but not another and you suspect a missing variable or uninstalled pack._

  ```bash
  rewst-cli drift --org 11111111-1111-1111-1111-111111111111 --against 22222222-2222-2222-2222-222222222222 --agent
  ```
- **`coverage`**  -  Across a parent org's managed sub-orgs, shows which integration packs are installed where and flags orgs missing a pack  -  bounded by a scan cap.

  _Use this to confirm an integration is rolled out to every client tenant, or to find the ones it skipped._

  ```bash
  rewst-cli coverage --parent 11111111-1111-1111-1111-111111111111 --pack microsoft --agent
  ```

## Command Reference

**action-options**  -  Manage action-options

- `rewst-cli action-options create`  -  Create a actionoption
- `rewst-cli action-options get`  -  Get a single actionoption

**actions**  -  Manage actions

- `rewst-cli actions create`  -  Create a action
- `rewst-cli actions get`  -  Get a single action

**active-conversation-requests**  -  Manage active-conversation-requests

- `rewst-cli active-conversation-requests`  -  Get a single activeconversationrequest

**affected-workflow-infos**  -  Manage affected-workflow-infos

- `rewst-cli affected-workflow-infos`  -  Get a single affectedworkflowinfo

**api-client-lists**  -  Manage api-client-lists

- `rewst-cli api-client-lists`  -  Get a single apiclientlist

**api-clients**  -  Manage api-clients

- `rewst-cli api-clients <id>`  -  Get a single apiclient

**app-platform-reserved-domains**  -  Manage app-platform-reserved-domains

- `rewst-cli app-platform-reserved-domains create`  -  Create a appplatformreserveddomain
- `rewst-cli app-platform-reserved-domains get`  -  Get a single appplatformreserveddomain
- `rewst-cli app-platform-reserved-domains update`  -  Update a appplatformreserveddomain

**commonly-used-actions**  -  Manage commonly-used-actions

- `rewst-cli commonly-used-actions`  -  Get a single commonlyusedaction

**component-instances**  -  Manage component-instances

- `rewst-cli component-instances create`  -  Create a componentinstance
- `rewst-cli component-instances get`  -  Get a single componentinstance
- `rewst-cli component-instances update`  -  Update a componentinstance

**component-trees**  -  Manage component-trees

- `rewst-cli component-trees`  -  Get a single componenttree

**components**  -  Manage components

- `rewst-cli components`  -  Get a single component

**conversation-message-votes**  -  Manage conversation-message-votes

- `rewst-cli conversation-message-votes create`  -  Create a conversationmessagevote
- `rewst-cli conversation-message-votes get`  -  Get a single conversationmessagevote
- `rewst-cli conversation-message-votes update`  -  Update a conversationmessagevote

**conversation-messages**  -  Manage conversation-messages

- `rewst-cli conversation-messages`  -  Create a conversationmessage

**conversations**  -  Manage conversations

- `rewst-cli conversations create`  -  Create a conversation
- `rewst-cli conversations get`  -  Get a single conversation
- `rewst-cli conversations update`  -  Update a conversation

**crate-override-options**  -  Manage crate-override-options

- `rewst-cli crate-override-options create`  -  Create a crateoverrideoption
- `rewst-cli crate-override-options update`  -  Update a crateoverrideoption

**crate-overrides**  -  Manage crate-overrides

- `rewst-cli crate-overrides create`  -  Create a crateoverride
- `rewst-cli crate-overrides update`  -  Update a crateoverride

**crate-unpacking-argument-sets**  -  Manage crate-unpacking-argument-sets

- `rewst-cli crate-unpacking-argument-sets`  -  Get a single crateunpackingargumentset

**crate-use-cases**  -  Manage crate-use-cases

- `rewst-cli crate-use-cases`  -  Get a single crateusecase

**crates**  -  Manage crates

- `rewst-cli crates create`  -  Create a crate
- `rewst-cli crates get`  -  Get a single crate
- `rewst-cli crates update`  -  Update a crate

**database-notification-errors**  -  Manage database-notification-errors

- `rewst-cli database-notification-errors`  -  Get a single databasenotificationerror

**delete-org-interpreter-responses**  -  Manage delete-org-interpreter-responses

- `rewst-cli delete-org-interpreter-responses <id>`  -  Delete a deleteorginterpreterresponse

**dropdown-options**  -  Manage dropdown-options

- `rewst-cli dropdown-options`  -  Get a single dropdownoption

**encoded-page-nodeses**  -  Manage encoded-page-nodeses

- `rewst-cli encoded-page-nodeses <id>`  -  Get a single encodedpagenodes

**feature-preview-settings**  -  Manage feature-preview-settings

- `rewst-cli feature-preview-settings create`  -  Create a featurepreviewsetting
- `rewst-cli feature-preview-settings get`  -  Get a single featurepreviewsetting
- `rewst-cli feature-preview-settings update`  -  Update a featurepreviewsetting

**foreign-object-references**  -  Manage foreign-object-references

- `rewst-cli foreign-object-references`  -  Get a single foreignobjectreference

**form-audit-entries**  -  Manage form-audit-entries

- `rewst-cli form-audit-entries`  -  Get a single formauditentry

**form-permission-states**  -  Manage form-permission-states

- `rewst-cli form-permission-states`  -  Get a single formpermissionstate

**forms**  -  Manage forms

- `rewst-cli forms create`  -  Create a form
- `rewst-cli forms get`  -  Get a single form
- `rewst-cli forms update`  -  Update a form

**integration-workflow-outputs**  -  Manage integration-workflow-outputs

- `rewst-cli integration-workflow-outputs`  -  Create a integrationworkflowoutput

**integrations**  -  Manage integrations

- `rewst-cli integrations`  -  Get a single integration

**interpreter-versions**  -  Manage interpreter-versions

- `rewst-cli interpreter-versions`  -  Get a single interpreterversion

**jinja-render-sessions**  -  Manage jinja-render-sessions

- `rewst-cli jinja-render-sessions <id>`  -  Get a single jinjarendersession

**jinja2-documentations**  -  Manage jinja2-documentations

- `rewst-cli jinja2-documentations`  -  Get a single jinja2documentation

**job-requested-responses**  -  Manage job-requested-responses

- `rewst-cli job-requested-responses`  -  Create a jobrequestedresponse

**logins**  -  Manage logins

- `rewst-cli logins`  -  Get a single login

**message-vote-statses**  -  Manage message-vote-statses

- `rewst-cli message-vote-statses`  -  Get a single messagevotestats

**microsoft-c-s-p-customers**  -  Manage microsoft-c-s-p-customers

- `rewst-cli microsoft-c-s-p-customers`  -  Get a single microsoftcspcustomer

**monaco-completion-items**  -  Manage monaco-completion-items

- `rewst-cli monaco-completion-items`  -  Get a single monacocompletionitem

**onboarding-questionnaire-responses**  -  Manage onboarding-questionnaire-responses

- `rewst-cli onboarding-questionnaire-responses create`  -  Create a onboardingquestionnaireresponse
- `rewst-cli onboarding-questionnaire-responses get`  -  Get a single onboardingquestionnaireresponse

**org-breadcrumbs**  -  Manage org-breadcrumbs

- `rewst-cli org-breadcrumbs`  -  Get a single orgbreadcrumb

**org-form-field-instances**  -  Manage org-form-field-instances

- `rewst-cli org-form-field-instances`  -  Get a single orgformfieldinstance

**org-interpreter-settings**  -  Manage org-interpreter-settings

- `rewst-cli org-interpreter-settings`  -  Get a single orginterpretersetting

**org-search-results**  -  Manage org-search-results

- `rewst-cli org-search-results`  -  Get a single orgsearchresult

**org-support-accesses**  -  Manage org-support-accesses

- `rewst-cli org-support-accesses`  -  Create a orgsupportaccess

**org-trigger-instances**  -  Manage org-trigger-instances

- `rewst-cli org-trigger-instances get`  -  Get a single orgtriggerinstance
- `rewst-cli org-trigger-instances update`  -  Update a orgtriggerinstance

**org-variables**  -  Manage org-variables

- `rewst-cli org-variables create`  -  Create a orgvariable
- `rewst-cli org-variables get`  -  Get a single orgvariable
- `rewst-cli org-variables update`  -  Update a orgvariable

**organization-audit-entries**  -  Manage organization-audit-entries

- `rewst-cli organization-audit-entries`  -  Get a single organizationauditentry

**organization-imports**  -  Manage organization-imports

- `rewst-cli organization-imports`  -  Get a single organizationimport

**organization-onboarding-crate-requirements**  -  Manage organization-onboarding-crate-requirements

- `rewst-cli organization-onboarding-crate-requirements`  -  Get a single organizationonboardingcraterequirement

**organization-onboarding-pack-requirements**  -  Manage organization-onboarding-pack-requirements

- `rewst-cli organization-onboarding-pack-requirements`  -  Get a single organizationonboardingpackrequirement

**organization-onboarding-requirements**  -  Manage organization-onboarding-requirements

- `rewst-cli organization-onboarding-requirements create`  -  Create a organizationonboardingrequirement
- `rewst-cli organization-onboarding-requirements get`  -  Get a single organizationonboardingrequirement

**organizations**  -  Manage organizations

- `rewst-cli organizations create`  -  Create a organization
- `rewst-cli organizations get`  -  Get a single organization
- `rewst-cli organizations update`  -  Update a organization

**pack-action-options**  -  Manage pack-action-options

- `rewst-cli pack-action-options`  -  Get a single packactionoption

**pack-bundles**  -  Manage pack-bundles

- `rewst-cli pack-bundles`  -  Get a single packbundle

**pack-configs**  -  Manage pack-configs

- `rewst-cli pack-configs create`  -  Create a packconfig
- `rewst-cli pack-configs get`  -  Get a single packconfig
- `rewst-cli pack-configs update`  -  Update a packconfig

**pack-delete-responses**  -  Manage pack-delete-responses

- `rewst-cli pack-delete-responses`  -  Delete a packdeleteresponse

**pack-resource-types-containers**  -  Manage pack-resource-types-containers

- `rewst-cli pack-resource-types-containers`  -  Get a single packresourcetypescontainer

**packs**  -  Manage packs

- `rewst-cli packs create`  -  Create a pack
- `rewst-cli packs get`  -  Get a single pack
- `rewst-cli packs update`  -  Update a pack

**packs-and-bundles-by-installed-states**  -  Manage packs-and-bundles-by-installed-states

- `rewst-cli packs-and-bundles-by-installed-states`  -  Get a single packsandbundlesbyinstalledstate

**page-nodes**  -  Manage page-nodes

- `rewst-cli page-nodes`  -  Get a single pagenode

**pages**  -  Manage pages

- `rewst-cli pages create`  -  Create a page
- `rewst-cli pages get`  -  Get a single page
- `rewst-cli pages update`  -  Update a page

**pending-tasks-aggregates**  -  Manage pending-tasks-aggregates

- `rewst-cli pending-tasks-aggregates`  -  Get a single pendingtasksaggregate

**permission-audit-log-lists**  -  Manage permission-audit-log-lists

- `rewst-cli permission-audit-log-lists`  -  Get a single permissionauditloglist

**permissions**  -  Manage permissions

- `rewst-cli permissions create`  -  Create a permission
- `rewst-cli permissions get`  -  Get a single permission
- `rewst-cli permissions update`  -  Update a permission

**psa-filter-optionses**  -  Manage psa-filter-optionses

- `rewst-cli psa-filter-optionses`  -  Get a single psafilteroptions

**psa-organizations**  -  Manage psa-organizations

- `rewst-cli psa-organizations`  -  Get a single psaorganization

**public-crates**  -  Manage public-crates

- `rewst-cli public-crates`  -  Get a single publiccrate

**reserved-organization-names**  -  Manage reserved-organization-names

- `rewst-cli reserved-organization-names`  -  Get a single reservedorganizationname

**robo-rewsty-config-values**  -  Manage robo-rewsty-config-values

- `rewst-cli robo-rewsty-config-values`  -  Get a single roborewstyconfigvalue

**role-organization-counts**  -  Manage role-organization-counts

- `rewst-cli role-organization-counts`  -  Get a single roleorganizationcount

**role-organization-rows**  -  Manage role-organization-rows

- `rewst-cli role-organization-rows`  -  Get a single roleorganizationrow

**role-user-counts**  -  Manage role-user-counts

- `rewst-cli role-user-counts`  -  Get a single roleusercount

**roles**  -  Manage roles

- `rewst-cli roles create`  -  Create a role
- `rewst-cli roles get`  -  Get a single role
- `rewst-cli roles update`  -  Update a role

**sensor-types**  -  Manage sensor-types

- `rewst-cli sensor-types`  -  Get a single sensortype

**site-domain-valids**  -  Manage site-domain-valids

- `rewst-cli site-domain-valids`  -  Get a single sitedomainvalid

**sites**  -  Manage sites

- `rewst-cli sites create`  -  Create a site
- `rewst-cli sites get`  -  Get a single site
- `rewst-cli sites update`  -  Update a site

**spice-d-b-check-results**  -  Manage spice-d-b-check-results

- `rewst-cli spice-d-b-check-results`  -  Get a single spicedbcheckresult

**tags**  -  Manage tags

- `rewst-cli tags create`  -  Create a tag
- `rewst-cli tags get`  -  Get a single tag
- `rewst-cli tags update`  -  Update a tag

**task-count-by-dates**  -  Manage task-count-by-dates

- `rewst-cli task-count-by-dates`  -  Get a single taskcountbydate

**task-count-by-hours**  -  Manage task-count-by-hours

- `rewst-cli task-count-by-hours`  -  Get a single taskcountbyhour

**task-logs**  -  Manage task-logs

- `rewst-cli task-logs create`  -  Create a tasklog
- `rewst-cli task-logs get`  -  Get a single tasklog

**templates**  -  Manage templates

- `rewst-cli templates create`  -  Create a template
- `rewst-cli templates get`  -  Get a single template
- `rewst-cli templates update`  -  Update a template

**time-saved-by-dates**  -  Manage time-saved-by-dates

- `rewst-cli time-saved-by-dates`  -  Get a single timesavedbydate

**time-saved-by-hours**  -  Manage time-saved-by-hours

- `rewst-cli time-saved-by-hours`  -  Get a single timesavedbyhour

**time-saved-group-by-orgs**  -  Manage time-saved-group-by-orgs

- `rewst-cli time-saved-group-by-orgs`  -  Get a single timesavedgroupbyorg

**time-saved-group-by-workflows**  -  Manage time-saved-group-by-workflows

- `rewst-cli time-saved-group-by-workflows`  -  Get a single timesavedgroupbyworkflow

**trigger-types**  -  Manage trigger-types

- `rewst-cli trigger-types`  -  Get a single triggertype

**triggers**  -  Manage triggers

- `rewst-cli triggers create`  -  Create a trigger
- `rewst-cli triggers get`  -  Get a single trigger
- `rewst-cli triggers update`  -  Update a trigger

**user-delegated-accesses**  -  Manage user-delegated-accesses

- `rewst-cli user-delegated-accesses`  -  Get a single userdelegatedaccess

**user-favorite-actions**  -  Manage user-favorite-actions

- `rewst-cli user-favorite-actions create`  -  Create a userfavoriteaction
- `rewst-cli user-favorite-actions update`  -  Update a userfavoriteaction

**user-invites**  -  Manage user-invites

- `rewst-cli user-invites create`  -  Create a userinvite
- `rewst-cli user-invites get`  -  Get a single userinvite

**user-preferenceses**  -  Manage user-preferenceses

- `rewst-cli user-preferenceses`  -  Update a userpreferences

**user-robo-rewsty-preferenceses**  -  Manage user-robo-rewsty-preferenceses

- `rewst-cli user-robo-rewsty-preferenceses get`  -  Get a single userroborewstypreferences
- `rewst-cli user-robo-rewsty-preferenceses update`  -  Update a userroborewstypreferences

**users**  -  Manage users

- `rewst-cli users create`  -  Create a user
- `rewst-cli users get`  -  Get a single user
- `rewst-cli users update`  -  Update a user

**workflow-execution-statses**  -  Manage workflow-execution-statses

- `rewst-cli workflow-execution-statses`  -  Get a single workflowexecutionstats

**workflow-executions**  -  Manage workflow-executions

- `rewst-cli workflow-executions`  -  Get a single workflowexecution

**workflow-notes**  -  Manage workflow-notes

- `rewst-cli workflow-notes`  -  Get a single workflownote

**workflow-patches**  -  Manage workflow-patches

- `rewst-cli workflow-patches`  -  Get a single workflowpatch

**workflow-stats-by-orgs**  -  Manage workflow-stats-by-orgs

- `rewst-cli workflow-stats-by-orgs`  -  Get a single workflowstatsbyorg

**workflow-tasks**  -  Manage workflow-tasks

- `rewst-cli workflow-tasks`  -  Get a single workflowtask

**workflows**  -  Manage workflows

- `rewst-cli workflows create`  -  Create a workflow
- `rewst-cli workflows get`  -  Get a single workflow
- `rewst-cli workflows update`  -  Update a workflow


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
rewst-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Morning fleet check

```bash
rewst-cli health --org YOUR_ORG_ID --since 24h --agent
```

Single-call health verdict (succeeded/failed/running plus time saved) to start the day.

### Triage overnight failures

```bash
rewst-cli failures --org YOUR_ORG_ID --since 12h --limit 25 --agent
```

List just the failed executions, newest first, to investigate what broke.

### Narrow a verbose execution list

```bash
rewst-cli workflow-executions --where '{"orgId":"YOUR_ORG_ID"}' --limit 50 --agent --select data.id,data.status,data.createdAt,data.numSuccessfulTasks
```

Pull only the fields you need from a deeply nested execution payload so an agent isn't flooded with the full object.

### Report automation ROI

```bash
rewst-cli roi --org YOUR_ORG_ID --since 30d --agent
```

Roll up humanSecondsSaved into hours/days saved and the top time-saving workflows for a client report.

### Find config drift between two tenants

```bash
rewst-cli drift --org TENANT_A --against TENANT_B --agent
```

Diff org variables and installed packs to explain why automation works in one tenant but not the other.

## Auth Setup

Rewst authenticates with a per-organization API client token sent as `Authorization: Bearer <token>`. Mint a token in the Rewst platform under Configuration > API Clients, then export it as `REWST_API_TOKEN` (or run `rewst-cli auth set-token`). The token is org-scoped, so most commands take an `--org` id for the tenant you are operating on.

Run `rewst-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  rewst-cli action-options get --agent --select id,name,status
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
rewst-cli feedback "the --since flag is inclusive but docs say exclusive"
rewst-cli feedback --stdin < notes.txt
rewst-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/rewst-cli/feedback.jsonl`. They are never POSTed unless `REWST_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `REWST_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled agent calls the same command every run with the same configuration.

```
rewst-cli profile save briefing --json
rewst-cli --profile briefing action-options get
rewst-cli profile list --json
rewst-cli profile show briefing
rewst-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `rewst-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add rewst-mcp -- rewst-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which rewst-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   rewst-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `rewst-cli <command> --help`.
