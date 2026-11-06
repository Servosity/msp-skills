---
name: rewst
description: "Every Rewst entity as a typed command, plus fleet health, ROI, drift and coverage rollups. Trigger phrases: `check rewst automation health`, `rewst failed workflows`, `how much time did rewst save`, `rewst config drift between orgs`, `which rewst orgs are missing a pack`, `use rewst`, `run rewst`."
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
    install:
      - kind: script
        bins: [rewst-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rewst/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rewst/install.ps1
---

# Rewst - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `rewst-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rewst/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rewst/install.ps1 | iex
   ```
3. Verify: `rewst-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `rewst-cli` and `rewst-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Rewst's only surface is a GraphQL gateway with no first-party CLI and no local state. This CLI mirrors organizations, workflows, executions, variables, packs, triggers, forms, and the rest as typed commands with --json/--select/--agent output, the `api` command that lists every GraphQL interface and method, and offline SQLite sync. On top of that it adds multi-org rollups  -  fleet execution health, failure triage, automation ROI, dormant-workflow detection, cross-org config drift, and pack coverage  -  that an MSP operator otherwise has to assemble by clicking through the web app one client at a time.

## When to Use This CLI

Use this CLI to operate and monitor Rewst from the terminal or an agent: check whether automation is healthy for a client, triage failed runs, report how much time automation saved, find dormant workflows, compare configuration between tenants, and read or run any Rewst entity. It is the right tool when you want scriptable, JSON-first access to Rewst across many client organizations rather than clicking through the web console one org at a time.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI to build or visually edit workflow graphs  -  author those in the Rewst workflow designer.
- Do not use it as a long-running event listener or webhook receiver; for inbound triggers configure a Rewst trigger or the webhook URL instead.
- Do not use it to administer billing or the Rewst account itself; that lives in the Rewst console.

## Unique Capabilities

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
- **`orgs`**  -  The token's own organization and every organization it manages (id, name, managing org, enabled), sorted by name: the ids health, failures, roi, dormant, drift and coverage take.

  _Run it first: every other rollup needs an --org or --parent id, and without it an operator copies ids out of the web console one client at a time._

  ```bash
  rewst-cli orgs --agent
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

> **Known limitation of this build:** the typed entity commands below are generated as REST-shaped calls the Rewst GraphQL gateway refuses, so they fail with an HTTP 400 or a GraphQL error; a refused or errored answer is a failure, never empty data. The commands that reach Rewst are `orgs`, `health`, `failures`, `roi`, `dormant`, `drift`, `coverage` and `doctor`: use them.

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


**coverage**


- `rewst-cli coverage`  -  Across a parent org's managed sub-orgs, shows which integration packs are installed where and flags orgs missing a pack  -  bounded by a scan cap.


**dormant**


- `rewst-cli dormant`  -  Workflows that have not executed in N days  -  dead or orphaned automation that is still installed but no longer running.


**drift**


- `rewst-cli drift`  -  Compares org variables and installed packs between two organizations and reports what one has that the other is missing.


**failures**


- `rewst-cli failures`  -  Recent failed workflow executions for an org, newest first, with workflow id, status, task counts, and age  -  the triage queue the API has no single endpoint for.


**health**


- `rewst-cli health`  -  One-call workflow execution health for an org: succeeded / failed / running / pending counts plus time saved, with a clear unhealthy verdict when failures are present.


**roi**


- `rewst-cli roi`  -  Aggregates Rewst's humanSecondsSaved metric across an org's workflows into total hours/days saved and the top time-saving automations.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
rewst-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

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

### Find the org ids this token can see

```bash
rewst-cli orgs --agent --select managed_orgs.id,managed_orgs.name
```

One GraphQL read of the token's organization and every org it manages; feed the ids to --org (health, failures, roi, dormant, drift) or --parent (coverage).

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

## Credential safety

- The API token (`Authorization: Bearer <REWST_API_TOKEN>`) and every API request go only to the scheme, host and port of the configured base URL (`https://api.rewst.io`, or `REWST_BASE_URL` for the EU and AU regions; every operation is `POST /graphql` there). A redirect to any other host is refused, so the token never reaches it.
- Any output, log, file, local-store row or MCP result that would repeat `REWST_API_TOKEN`, the stored token, or a credential a response carries (an API client's `auth0ClientSecret` and `auth0ClientId`, a third-party login token, an integration secret inside a pack config) is replaced by `[credkit: withheld from <where>: it carried a credential this program holds]`, in plain text and in the encodings the shared credential kit decodes. Responses are never cached on disk.
- A `--stdin` body is held whole, so a write's answer that repeats part of the body prints that notice too.
- Not held yet: the value of an org variable whose category is `secret` prints as Rewst returns it. Treat `org-variables` output as sensitive.
- Only `credentials.toml` (mode 0600) holds a credential; `auth set-token` reads the token from stdin, never from an argument.
- A GraphQL answer that carries an `errors` member is reported as a failure, never as empty data.
- `--dry-run` sends nothing. Over MCP, `rewst_execute` (every Rewst operation that changes something), `import` and `jobs_prune` return a preview and send only with `confirm: true`, and a tool argument can never name a file to read or write.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, sync, and `--deliver` paths:

- `--json`  -  one JSON document on stdout (sync progress events go to stderr)
- `--compact`  -  keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain`  -  tabular rows (collection envelopes unwrap to the row array). `--csv` prefixes `'` on a cell or header key that starts with `=`, `+`, `-`, `@`, tab, CR, or LF (fullwidth `＝＋－＠` too) so a spreadsheet does not run it as a formula; a plain signed number such as `-5` or `+2.5e3` is unchanged.
- `--quiet`  -  one identity value per row, no envelope

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  rewst-cli action-options get --agent --select actions,id,optionLabel
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit confirmation**  -  `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
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

- Use `--home <dir>` for one invocation, or set `REWST_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `REWST_CONFIG_DIR`, `REWST_DATA_DIR`, `REWST_STATE_DIR`, `REWST_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `REWST_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `rewst-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "rewst": {
        "command": "rewst-mcp",
        "env": {
          "REWST_HOME": "/srv/rewst"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `REWST_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `REWST_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are never cached on disk. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `REWST_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `REWST_DATA_SOURCE` sets the `--data-source` default.

## Automatic learning

This CLI ships a self-capturing learning loop. The CLI does its own bookkeeping: every invocation is journaled locally, a failed flag followed by a corrected retry auto-derives a `flag_alias` candidate, and a `teach` on a query family without a playbook auto-synthesizes a `playbook_candidate` from the session's journal. Your job is judgment only: `recall` first, act on surfaced candidates, `teach` the final answer, `playbook amend` when you observe a correction. You never record failures by hand.

### Step 1: `recall` before any discovery

Before list/search/drill commands on a new user question, pass the question as an argv or MCP tool argument to `recall --agent`. Do not interpolate user-controlled text into a shell command line.

Quoted `recall "<question>"` breaks on an apostrophe, which is ordinary English. A quoted heredoc breaks when a body line equals the delimiter, and that delimiter is published in these docs. Write the question with a non-shell file-writing tool, then read it back as data:

```bash
# Write the question verbatim with your file-writing tool (no shell involved).
# Command substitution on a file only ever yields data  -  the shell never
# parses the file's bytes as syntax.
QUERY=$(cat /path/to/question.txt)
rewst-cli recall "$QUERY" --agent
```

Prefer MCP: pass the question as the tool's query argument. `"$QUERY"` after a file read is argv-safe; putting the question itself in the command text is not.

The response envelope:

```json
{
  "query": "...",
  "normalized": "<normalized form>",
  "query_entities": ["..."],
  "found": true | false,
  "match_score": 0.0,
  "results": [
    { "resource_id": "...", "resource_type": "...", "venue": "...",
      "confidence": 2, "entity_match": "exact|partial|unknown",
      "source": "taught|preseed|pattern", "warnings": ["..."] }
  ],
  "mismatches": [ /* only when --debug-mismatches */ ],
  "warnings": [ /* top-level */ ],
  "candidates": [
    { "id": 12, "class": "flag_alias | playbook_candidate",
      "summary": "...", "sightings": 3, "last_seen": "...",
      "rationale": "...",
      "next_action": ["<trial command>", "rewst-cli learnings confirm 12"] }
  ],
  "playbook": {
    "query_family": "...",
    "playbook": {
      "steps": [ { "cmd": "<command with {slot} substitution>", "purpose": "..." } ],
      "entity_slots": ["$ENTITY"],
      "expected_tool_calls": 3
    },
    "slots_resolved": { "$ENTITY": { "token": "<live token>", "canonical": "<canonical>" } },
    "notes": "<workarounds + gotchas for this query family>"
  },
  "notes": "<duplicate surface for non-playbook callers>"
}
```

Empty-store short-circuit: if the store has no learnings, playbooks, or candidates yet (recall finds nothing and `learnings list` and `learnings candidates` are both empty), skip recall for the rest of this session instead of taxing every query; resume recall-first once something has been taught.

### Step 2: decision tree

Read `candidates`, `playbook`, `notes`, `results[0]`, and warnings in that order:

```
if Candidates present (warnings include "candidates_present"):
    -> candidates are try-then-confirm, never facts. Follow each candidate's
       two-step next_action verbatim: run the trial command first, then run
       `learnings confirm <id>` only after the trial verified the behavior.
       Reject a wrong candidate with `learnings reject <id>`.
    -> NEVER re-teach something recall surfaced as a candidate; confirm or
       reject that candidate instead of teaching a duplicate.
    -> candidates ride alongside playbooks and resource hits, not instead of
       them; continue with the branches below after acting on them.

if Playbook present:
    -> READ Playbook.notes verbatim FIRST (workarounds + gotchas the CLI surface doesn't expose)
    -> replay Playbook.steps in order, substituting Playbook.slots_resolved entries
       for the entity slot tokens. If a step's slot is unresolved, fall back to
       discovery for that step only.
    -> the Playbook's expected_tool_calls is a budget; if you find yourself running
       materially more, record the divergence via `rewst-cli playbook amend`
       at end-of-session.

elif Notes present (no Playbook):
    -> read Notes verbatim before any discovery step; they carry known gotchas
       for this query family even when no structured choreography exists yet.

elif Found AND Results[0].EntityMatch == "exact" AND Results[0].Confidence >= 2:
    -> skip discovery; fetch live data for Results[*].ResourceID in parallel

elif Found AND Results[0].EntityMatch == "partial":
    -> candidate hint, NOT a hit; read the resource title to validate before trusting

elif (any row in Mismatches[] when --debug-mismatches was passed):
    -> treat as cold start; the stored learning is for a different entity
       (different canonical resolved from query_entities)

else:  // Found == false, no playbook, no notes
    -> cold start; run discovery normally; teach the answer afterward (Step 4).
       If the family has no playbook yet, that teach auto-synthesizes a
       playbook candidate from this session's journal - you do not need to
       record one by hand.
```

Playbook and Notes are orthogonal to the per-resource path. A recall response can carry both a Playbook AND a `Results[]` hit - use both: the Playbook tells you which choreography to run; the resource hits short-circuit specific steps. Default to skipping `mismatches`; pass `--debug-mismatches` only when investigating cold-start surprises.

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `rewst-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `rewst-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
rewst-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
rewst-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
rewst-cli teach-playbook \
  --query "$QUERY" \
  --playbook-file ~/playbooks/<shape>.json \
  --notes-file ~/playbooks/<shape>-notes.md
```

Playbook files are JSON with `steps`, `entity_slots`, `expected_tool_calls`. Notes files are markdown carrying the gotchas verbatim. File-free callers (MCP-only agents) pass the same content inline: `--playbook-json` and `--playbook-notes` on the integrated `teach` form, `--playbook-json` and `--notes` on `teach-playbook`. On the integrated `teach` form, the playbook flags are optional - omit them entirely for a resource-only teach. On the standalone `teach-playbook` form, at least one of the playbook and notes flags must be set; both empty is rejected. Playbooks are keyed on the structural query family (entities stripped) so a recipe taught from one entity-shaped query applies to every other query of the same shape, with `slots_resolved` binding the live query's canonical at recall time.

When you DO find a playbook on a future recall, treat it as ground truth: replay the steps with `slots_resolved` substitutions, skip the discovery that the choreography already documents, and read `notes` before any step.

### Step 6: `playbook amend &` when your debug response identifies a correction

If your debug-protocol response identifies a concrete correction the notes or playbook should know  -  a workaround, an undocumented endpoint shape, a stale field name, observed schema drift, an empty-payload fallback  -  fire `playbook amend` BEFORE emitting your user-facing response. Same fire-and-forget posture as `teach`. Pass the query and note as argv/MCP arguments, or write each with a non-shell file tool and read them back (`QUERY=$(cat ...)`, `NOTE=$(cat ...)`). Do not interpolate either string into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
NOTE=$(cat /path/to/note.txt)
rewst-cli playbook amend \
  --query "$QUERY" \
  --add-note "$NOTE"
# (append shell `&` to background it)
```

What counts as worth amending: a behavior you OBSERVED this session that future-you would benefit from knowing. Examples worth amending:

- A workaround for a CLI surface that silently drops or misorders a flag.
- An undocumented endpoint shape (response wrapped in `{meta, results}`, payload nested two levels deeper than the docs claim).
- Observed schema drift (a field renamed, an index that shifted between seasons, a category label that the API now returns lower-cased).

What does NOT belong in notes:

- The year-specific or entity-specific answer to the user's question. That's the response, not a learning.
- Per-team / per-athlete / per-row data the playbook already retrieves at runtime.
- Statements that paraphrase what the existing notes already say.

The amend command appends to the family's existing notes with a timestamped marker (`[amend YYYY-MM-DDTHH:MMZ]: <text>`). Multiple amends accumulate; the audit trail is visible. If no playbook exists yet for the family, amend creates a notes-only one (so cold-start corrections still land).

#### PII discipline for amend notes

`playbook amend` notes are designed to potentially flow upstream as shared knowledge in future versions of the Printing Press. Keep them clean of user-identifying content so the upstream-contribution path stays open without retroactive scrubbing:

- **Do NOT embed** paths to user filesystems, personal API keys or tokens, user email addresses, user GitHub handles, or specific query histories tied to a single user.
- **Acceptable**: endpoint shapes, undocumented field names, API gotchas, observed schema drift, workarounds for CLI surfaces, generalizable pagination or retry tactics.

If a correction is only meaningful with user-specific context, it belongs in a personal note, not in the playbook amend.

### Measuring the loop

`rewst-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `REWST_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
rewst-cli feedback "the --since flag is inclusive but docs say exclusive"
rewst-cli feedback --stdin < notes.txt
rewst-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `REWST_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `REWST_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` as a private 0600 file (exclusive randomly named temp in the destination directory, then rename; a planted symlink is not followed). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`). Redirects are not followed; a 3xx response fails the delivery and the body is not sent to the `Location`. |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures, including redirects, return non-zero and log the URL and HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

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

If `--wait` times out or polling fails after the job was accepted, the command exits `8` and prints the job ID plus a recovery command that fetches the result. The job may still be running and may already be billed, so run the recovery command instead of resubmitting.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 6 | Partial failure |
| 7 | Rate limited (wait and retry) |
| 8 | Async job submitted but not finished (run the printed recovery command; do not resubmit) |
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

For a remote host, `rewst-mcp --transport http` requires `REWST_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `REWST_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `REWST_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which rewst-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   rewst-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `rewst-cli <command> --help`.
