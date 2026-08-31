---
name: immybot
description: "Every ImmyBot endpoint typed, plus a local SQLite mirror that answers the cross-tenant questions the web UI cannot. Trigger phrases: `what failed in last night's maintenance window`, `which tenants are still on an old version of chrome`, `why didn't this computer get the deployment`, `what changed in the fleet since yesterday`, `which computers does this script reach`, `which machines are stuck onboarding`, `use immybot`, `run immybot`."
author: "Abhi Saini"
license: "Apache-2.0"
vendor: "ImmyBot"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - immybot-cli
    install:
      - kind: script
        bins: [immybot-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/immybot/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/immybot/install.ps1
---

# ImmyBot - Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `immybot-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/immybot/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/immybot/install.ps1 | iex
   ```
3. Verify: `immybot-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer places the `immybot-cli` and `immybot-mcp` binaries on your PATH. It does not
register anything with your agent - see [mcp-install.md](./mcp-install.md) for the
MCP wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

ImmyBot's API is large and entirely per-tenant, so the questions MSPs actually ask span calls the web UI never joins. This CLI types the whole surface, mirrors it into local SQLite with full-text search, and adds commands built on that mirror: session-triage collapses a night of failures into distinct root causes, version-spread ranks one software title across every tenant with a real semver comparator, and assignment-explain shows which deployment rule actually won on a given machine.

## When to Use This CLI

Use this CLI for any ImmyBot question that spans more than one tenant, or that needs history the API does not keep. It is the right tool for post-maintenance triage, cross-tenant software version audits, working out why a deployment did or did not land on a machine, and reconciling ImmyBot against a linked PSA or RMM. It is also the fastest way to script against the full ImmyBot surface, since every endpoint is typed and every command speaks JSON.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI to remote-control a user's desktop; ImmyBot's own remote control session is a browser workflow and this CLI only returns the screen-share URL.
- Do not use this CLI to author or debug PowerShell script content; it manages script records and execution history, not the script editor experience.
- Do not use this CLI as a live monitoring dashboard; cross-tenant commands read a local mirror that is only as fresh as the last sync.
- Do not use this CLI to configure Entra ID app registrations or ImmyBot user permissions for the first time; that setup is a portal workflow.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Local state that compounds
- **`session-triage`** - Group last night's failed maintenance actions by root cause instead of reading the same error on forty machines.

  _Reach for this first after any maintenance window: it turns N red machines into the handful of distinct problems actually worth a ticket._

  ```bash
  immybot-cli session-triage --since 24h --agent
  ```
- **`version-spread`** - Semver-ordered distribution of one software title across every tenant, flagging everything below a floor.

  _This is the CVE-response command: one call answers which clients are still exposed on a given title._

  ```bash
  immybot-cli version-spread "Google Chrome" --min-version 140 --agent
  ```
- **`fleet-diff`** - What actually changed between two syncs: computers added or removed, software versions moved, assignments modified. Requires a local baseline: run `fleet-diff --snapshot` after a sync to record one (there is no updated-since cursor in the API), then compare with `--since`.

  _Use this to answer "what changed since last night" without diffing two exports by hand._

  ```bash
  immybot-cli fleet-diff --since 24h --agent
  ```
- **`onboarding-stalled`** - Computers stuck waiting to onboard, bucketed by age and annotated with whether onboarding was ever attempted.

  _Surfaces machines that silently never finished onboarding, which is the failure mode clients notice first._

  ```bash
  immybot-cli onboarding-stalled --older-than 3d --agent
  ```

### Deployment resolution
- **`assignment-explain`** - Show every target assignment that resolves onto one computer, which scope matched, and which rules are shadowed.

  _Use this for any "why didn't this machine get X" question; it answers what a computer receives and why, which no single endpoint does._

  ```bash
  immybot-cli assignment-explain 4821 --agent
  ```
- **`script-blast-radius`** - Every maintenance task, software package, and computer that a script reaches before you edit it.

  _Run this before editing any shared script; it is the only way to see downstream reach across tenants._

  ```bash
  immybot-cli script-blast-radius 312 --agent
  ```

### Integration hygiene
- **`psa-reconcile`** - Diff the ImmyBot roster against a linked PSA or RMM asset roster to find unlinked computers and orphaned assets.

  _Run after each week of onboards and decommissions; mapping gaps otherwise surface as a wrong invoice or a machine that stopped getting maintenance._

  ```bash
  immybot-cli psa-reconcile --provider 7 --agent
  ```

## Command Reference

**access** - Manage access

- `immybot-cli access create-delete-azure-tenant-auth-details` - Create delete azure tenant auth details
- `immybot-cli access create-request` - Create request
- `immybot-cli access create-update-azure-tenant-auth-details` - Create update azure tenant auth details
- `immybot-cli access get-get-azure-tenant-auth-details-by-azure-tenant-principal-id` - Get get azure tenant auth details by azure tenant principal id
- `immybot-cli access get-get-ip-addresses` - Get get ip addresses
- `immybot-cli access get-me-permissions-by-permission-type-tenants` - Get me permissions by permission type tenants
- `immybot-cli access list` - List

**application-locks** - Manage application locks

- `immybot-cli application-locks create-request-cancellation` - Create request cancellation
- `immybot-cli application-locks get-realtime-event-stream` - Get realtime event stream
- `immybot-cli application-locks list` - List

**application-logs** - Manage application logs

- `immybot-cli application-logs create-source-context` - Create source context
- `immybot-cli application-logs create-source-context-clear` - Create source context clear
- `immybot-cli application-logs create-source-context-clear-all` - Create source context clear all
- `immybot-cli application-logs create-streaming` - Create streaming
- `immybot-cli application-logs get-source-contexts` - Get source contexts

**audits** - Manage audits

- `immybot-cli audits get-global-dx` - Get global dx
- `immybot-cli audits get-local-dx` - Get local dx

**azure** - Manage azure

- `immybot-cli azure create-disambiguate-tenant-type` - Create disambiguate tenant type
- `immybot-cli azure create-preconsent-customer-tenants` - Create preconsent customer tenants
- `immybot-cli azure create-sync-details-from-tenants` - Create sync details from tenants
- `immybot-cli azure create-sync-users-from-tenants` - Create sync users from tenants
- `immybot-cli azure create-tenant-consented` - Create tenant consented
- `immybot-cli azure get-app-registration-options` - Get app registration options
- `immybot-cli azure get-partner-tenant-customers-by-partner-principal-id` - Get partner tenant customers by partner principal id
- `immybot-cli azure get-partner-tenant-infos` - Get partner tenant infos

**azure-errors** - Manage azure errors

- `immybot-cli azure-errors get-dx` - Get dx
- `immybot-cli azure-errors get-for-tenant-by-tenant-principal-id-dx` - Get for tenant by tenant principal id dx

**billing** - Manage billing

- `immybot-cli billing create-cancel-subscription` - Create cancel subscription
- `immybot-cli billing create-information` - Create information
- `immybot-cli billing create-reactivate-subscription` - Create reactivate subscription
- `immybot-cli billing create-update-addon` - Create update addon
- `immybot-cli billing create-update-subscription` - Create update subscription
- `immybot-cli billing get-credit-cards` - Get credit cards
- `immybot-cli billing get-download-invoice` - Get download invoice
- `immybot-cli billing get-feature-usage-counts` - Get feature usage counts
- `immybot-cli billing get-information` - Get information
- `immybot-cli billing get-platform-details` - Get platform details
- `immybot-cli billing get-product-catalog` - Get product catalog
- `immybot-cli billing get-product-catalog-items` - Get product catalog items
- `immybot-cli billing get-subscription-details` - Get subscription details

**brandings** - Manage brandings

- `immybot-cli brandings create` - Create
- `immybot-cli brandings create-global-default-by-id` - Create global default by id
- `immybot-cli brandings create-send-test-email` - Create send test email
- `immybot-cli brandings create-validate-time-format-by-time-format` - Create validate time format by time format
- `immybot-cli brandings delete-by-id` - Delete by id
- `immybot-cli brandings get-by-id` - Get by id
- `immybot-cli brandings get-support` - Fetches support related branding changes to be used in the Support Sidebar, Session Support Request
- `immybot-cli brandings list` - List
- `immybot-cli brandings update-by-id` - Update by id

**change-requests** - Manage change requests

- `immybot-cli change-requests delete-by-id` - Delete by id
- `immybot-cli change-requests get-dx` - Get dx
- `immybot-cli change-requests get-open-count` - Get open count

**chocolatey** - Manage chocolatey

- `immybot-cli chocolatey get-find-packages-by-id` - Get find packages by id
- `immybot-cli chocolatey get-search` - Get search

**computers** - Manage computers

- `immybot-cli computers create-add-tags` - Create add tags
- `immybot-cli computers create-bulk-delete` - Create bulk delete
- `immybot-cli computers create-change-tenant` - Create change tenant
- `immybot-cli computers create-remove-tags` - Create remove tags
- `immybot-cli computers create-restore` - Create restore
- `immybot-cli computers create-set-excluded-from-user-affinity` - Create set excluded from user affinity
- `immybot-cli computers create-skip-onboarding` - Create skip onboarding
- `immybot-cli computers get-agent-status` - Get agent status
- `immybot-cli computers get-by-id` - Get by id
- `immybot-cli computers get-dx` - Get dx
- `immybot-cli computers get-export` - Get export
- `immybot-cli computers get-inventory` - Get inventory
- `immybot-cli computers get-inventory-export` - Get inventory export
- `immybot-cli computers get-inventory-software-search-by-name` - Get inventory software search by name
- `immybot-cli computers get-inventory-software-search-by-upgrade-code` - Get inventory software search by upgrade code
- `immybot-cli computers get-my` - Get my
- `immybot-cli computers get-onboarding` - Get onboarding
- `immybot-cli computers get-paged` - List computers with server-side paging, filtering, and sorting (skip/take)
- `immybot-cli computers get-user-affinities` - Get user affinities
- `immybot-cli computers get-user-affinities-export` - Get user affinities export
- `immybot-cli computers list` - List
- `immybot-cli computers update-by-id` - Update by id

**dynamic-provider-types** - Manage dynamic provider types

- `immybot-cli dynamic-provider-types create-global` - Create global
- `immybot-cli dynamic-provider-types create-global-by-id` - Create global by id
- `immybot-cli dynamic-provider-types create-global-by-id-reload` - Create global by id reload
- `immybot-cli dynamic-provider-types create-local` - Create local
- `immybot-cli dynamic-provider-types create-local-by-id` - Create local by id
- `immybot-cli dynamic-provider-types create-local-by-id-reload` - Create local by id reload
- `immybot-cli dynamic-provider-types create-reload` - Create reload
- `immybot-cli dynamic-provider-types create-test-environment-by-terminal-id` - Create test environment by terminal id
- `immybot-cli dynamic-provider-types create-test-environment-by-terminal-id-bind-configuration-form` - Create test environment by terminal id bind configuration form
- `immybot-cli dynamic-provider-types create-test-environment-by-terminal-id-execute-method-by-method` - Create test environment by terminal id execute method by method
- `immybot-cli dynamic-provider-types delete-global-by-id` - Delete global by id
- `immybot-cli dynamic-provider-types delete-local-by-id` - Delete local by id
- `immybot-cli dynamic-provider-types delete-test-environment-by-terminal-id` - Delete test environment by terminal id
- `immybot-cli dynamic-provider-types get-global-by-id` - Get global by id
- `immybot-cli dynamic-provider-types get-local-by-id` - Get local by id
- `immybot-cli dynamic-provider-types list` - List

**effective-permissions** - Manage effective permissions

- `immybot-cli effective-permissions create-groups-by-group-id-evaluate-all-assignments` - Returns all role assignments for a group grouped by permission without evaluation context.
- `immybot-cli effective-permissions create-groups-by-group-id-evaluate-resource` - Evaluates permissions for a group against a specific resource.
- `immybot-cli effective-permissions create-groups-by-group-id-evaluate-tenant` - Evaluates permissions for a group against a specific tenant.
- `immybot-cli effective-permissions create-users-by-user-id-evaluate-all-assignments` - Returns all role assignments for a user grouped by permission without evaluation context.
- `immybot-cli effective-permissions create-users-by-user-id-evaluate-resource` - Evaluates permissions for a user against a specific resource.
- `immybot-cli effective-permissions create-users-by-user-id-evaluate-tenant` - Evaluates permissions for a user against a specific tenant.

**ephemeral-session** - Manage ephemeral session

- `immybot-cli ephemeral-session get-by-agent-instance-id-by-provider-agent-id` - Get by agent instance id by provider agent id
- `immybot-cli ephemeral-session get-development-latest-ephemeral-binary` - Get development latest ephemeral binary
- `immybot-cli ephemeral-session get-development-latest-ephemeral-binary-v2` - Get development latest ephemeral binary v2

**getting-started** - Manage getting started

- `immybot-cli getting-started create-checklist-complete` - Create checklist complete
- `immybot-cli getting-started create-checklist-reset` - Create checklist reset
- `immybot-cli getting-started get-checklist` - Get checklist

**groups** - Manage groups

- `immybot-cli groups create` - Create
- `immybot-cli groups delete-by-id` - Delete by id
- `immybot-cli groups get-by-id` - Get by id
- `immybot-cli groups list` - List
- `immybot-cli groups update-by-id` - Update by id

**immy-agent-metadata** - Manage immy agent metadata

- `immybot-cli immy-agent-metadata` - Get agent hash

**installer** - Manage installer

- `immybot-cli installer` - Create agent rekey request

**inventory-tasks** - Manage inventory tasks

- `immybot-cli inventory-tasks create-local` - Create local
- `immybot-cli inventory-tasks create-local-by-id` - Create local by id
- `immybot-cli inventory-tasks create-local-by-id-scripts` - Create local by id scripts
- `immybot-cli inventory-tasks delete-local-by-id` - Delete local by id
- `immybot-cli inventory-tasks delete-local-by-task-id-scripts-by-inventory-key` - Delete local by task id scripts by inventory key
- `immybot-cli inventory-tasks list` - List

**licenses** - Manage licenses

- `immybot-cli licenses create` - Create
- `immybot-cli licenses create-upload` - Create upload
- `immybot-cli licenses delete-by-id` - Delete by id
- `immybot-cli licenses get-by-id` - Get by id
- `immybot-cli licenses get-dx` - Get dx
- `immybot-cli licenses list` - List
- `immybot-cli licenses update-by-id` - Update by id

**maintenance-actions** - Manage maintenance actions

- `immybot-cli maintenance-actions create-latest-action-for-computers` - Create latest action for computers
- `immybot-cli maintenance-actions create-latest-action-for-tenants` - Create latest action for tenants
- `immybot-cli maintenance-actions get-computer-by-computer-id-needs-attention` - Get computer by computer id needs attention
- `immybot-cli maintenance-actions get-dx` - Get dx
- `immybot-cli maintenance-actions get-dx-for-computer-by-computer-id` - Get dx for computer by computer id
- `immybot-cli maintenance-actions get-latest-for-computer-by-computer-id` - Get latest for computer by computer id
- `immybot-cli maintenance-actions get-latest-for-tenant-by-tenant-id` - Get latest for tenant by tenant id
- `immybot-cli maintenance-actions get-latest-non-compliant-actions-for-tenant-by-tenant-id` - Get latest non compliant actions for tenant by tenant id
- `immybot-cli maintenance-actions get-maintenance-item` - Get maintenance item
- `immybot-cli maintenance-actions get-version` - Get version

**maintenance-emails** - Manage maintenance emails


**maintenance-sessions** - Manage maintenance sessions

- `immybot-cli maintenance-sessions create-cancel` - Create cancel
- `immybot-cli maintenance-sessions create-cancel-all` - Create cancel all
- `immybot-cli maintenance-sessions create-rerun-v2` - Create rerun v2
- `immybot-cli maintenance-sessions get-by-session-id` - Get by session id
- `immybot-cli maintenance-sessions get-cancel-for-schedule-by-schedule-id` - Get cancel for schedule by schedule id
- `immybot-cli maintenance-sessions get-dx` - Get dx
- `immybot-cli maintenance-sessions get-status-counts` - Get status counts

**maintenance-tasks** - Manage maintenance tasks

- `immybot-cli maintenance-tasks create-duplicate` - Create duplicate
- `immybot-cli maintenance-tasks create-global` - Create global
- `immybot-cli maintenance-tasks create-global-by-id` - Create global by id
- `immybot-cli maintenance-tasks create-global-by-id-param-block-from-parameters` - Create global by id param block from parameters
- `immybot-cli maintenance-tasks create-local` - Create local
- `immybot-cli maintenance-tasks create-local-by-id` - Create local by id
- `immybot-cli maintenance-tasks create-local-by-id-migrate-local-to-global` - Create local by id migrate local to global
- `immybot-cli maintenance-tasks create-local-by-id-param-block-from-parameters` - Create local by id param block from parameters
- `immybot-cli maintenance-tasks create-validate-param-block-parameters` - Create validate param block parameters
- `immybot-cli maintenance-tasks delete-global-by-id` - Delete global by id
- `immybot-cli maintenance-tasks delete-local-by-id` - Delete local by id
- `immybot-cli maintenance-tasks get-global` - Get global
- `immybot-cli maintenance-tasks get-global-by-id` - Get global by id
- `immybot-cli maintenance-tasks get-local` - Get local
- `immybot-cli maintenance-tasks get-local-by-id` - Get local by id
- `immybot-cli maintenance-tasks get-local-by-id-migrate-local-to-global-what-if` - Get local by id migrate local to global what if
- `immybot-cli maintenance-tasks get-reference-count` - Get reference count
- `immybot-cli maintenance-tasks get-search` - Get search

**me** - Manage me

- `immybot-cli me` - Gets all role assignments and groups for the current user

**media** - Manage media

- `immybot-cli media create-global-by-id` - Create global by id
- `immybot-cli media create-global-upload` - Create global upload
- `immybot-cli media create-local-by-id` - Create local by id
- `immybot-cli media create-local-by-id-authorization` - Create local by id authorization
- `immybot-cli media create-local-upload` - Create local upload
- `immybot-cli media create-request-file-download-url` - Create request file download url
- `immybot-cli media create-support-upload` - Create support upload
- `immybot-cli media delete-global-by-id` - Delete global by id
- `immybot-cli media delete-local-by-id` - Delete local by id
- `immybot-cli media get-global` - Get global
- `immybot-cli media get-global-by-id` - Get global by id
- `immybot-cli media get-global-by-id-download-url` - Get global by id download url
- `immybot-cli media get-local` - Get local
- `immybot-cli media get-local-by-id` - Get local by id
- `immybot-cli media get-local-by-id-authorization` - Get local by id authorization
- `immybot-cli media get-local-by-id-download-url` - Get local by id download url
- `immybot-cli media get-search` - Get search

**metrics** - Manage metrics

- `immybot-cli metrics create-circuit-breakers-isolate` - Create circuit breakers isolate
- `immybot-cli metrics create-circuit-breakers-reset` - Create circuit breakers reset
- `immybot-cli metrics get-circuit-breakers` - Get circuit breakers
- `immybot-cli metrics get-provider-links` - Get provider links
- `immybot-cli metrics get-provider-links-by-provider-link-id-rate-limit-statistics` - Returns the current rate limiter statistics for a provider link. 200: stats available. 204: provider not initialized.

**notifications** - Manage notifications

- `immybot-cli notifications create-acknowledge` - Create acknowledge
- `immybot-cli notifications get-dx` - Get dx
- `immybot-cli notifications get-unacknowledged` - Get unacknowledged
- `immybot-cli notifications list` - List

**oauth** - Manage oauth

- `immybot-cli oauth create-access-tokens-by-id-refresh` - Create access tokens by id refresh
- `immybot-cli oauth create-begin-auth-code-flow` - Create begin auth code flow
- `immybot-cli oauth create-fail-auth-code-flow` - Create fail auth code flow
- `immybot-cli oauth create-finish-auth-code-flow` - Create finish auth code flow
- `immybot-cli oauth delete-access-tokens-by-id` - Delete access tokens by id
- `immybot-cli oauth get-access-tokens` - Get access tokens
- `immybot-cli oauth get-access-tokens-by-id-by-access-token-id` - Get access tokens by id by access token id

**persons** - Manage persons

- `immybot-cli persons create` - Create
- `immybot-cli persons create-add-tags` - Create add tags
- `immybot-cli persons create-remove-tags` - Create remove tags
- `immybot-cli persons delete-by-id` - Delete by id
- `immybot-cli persons get-by-id` - Get by id
- `immybot-cli persons get-dx` - Get dx
- `immybot-cli persons get-requesting-access` - Get requesting access
- `immybot-cli persons list` - List
- `immybot-cli persons update-by-id` - Update by id

**plugins** - Manage plugins

- `immybot-cli plugins create-api-v1-by-provider-link-id-by-catch-all` - Create api v1 by provider link id by catch all
- `immybot-cli plugins get-api-v1-by-provider-link-id-by-catch-all` - Get api v1 by provider link id by catch all
- `immybot-cli plugins get-api-v1-by-provider-link-id-by-catch-all-v2` - Get api v1 by provider link id by catch all v2

**preferences** - Manage preferences

- `immybot-cli preferences get-tenants-by-tenant-id` - Get tenants by tenant id
- `immybot-cli preferences list` - List
- `immybot-cli preferences update-application` - Update application
- `immybot-cli preferences update-my` - Update my
- `immybot-cli preferences update-tenants-by-tenant-id` - Update tenants by tenant id

**provider-agents** - Manage provider agents

- `immybot-cli provider-agents create-bulk-delete-pending` - Create bulk delete pending
- `immybot-cli provider-agents create-identify` - Identify agents that are marked with requiring manual identification
- `immybot-cli provider-agents create-resolve-failure-by-failure-id` - Create resolve failure by failure id
- `immybot-cli provider-agents create-resolve-failures` - Create resolve failures
- `immybot-cli provider-agents get-pending` - Get pending
- `immybot-cli provider-agents get-pending-counts` - Get pending counts

**provider-links** - Manage provider links

- `immybot-cli provider-links create` - Create
- `immybot-cli provider-links create-create-with-external-provider-reference` - Create create with external provider reference
- `immybot-cli provider-links create-verify-with-external-provider-reference` - Create verify with external provider reference
- `immybot-cli provider-links delete-by-id` - Delete by id
- `immybot-cli provider-links get-by-id` - Get by id
- `immybot-cli provider-links list` - List
- `immybot-cli provider-links update-by-id` - Update by id

**provider-types** - Manage provider types

- `immybot-cli provider-types get-client-group-types-by-client-group-type-id-client-groups` - Get client group types by client group type id client groups
- `immybot-cli provider-types get-device-group-types-by-device-group-type-id-device-groups` - Get device group types by device group type id device groups
- `immybot-cli provider-types get-form-dropdown-options-by-key` - Get form dropdown options by key
- `immybot-cli provider-types list` - List

**rmm-links** - Manage rmm links

- `immybot-cli rmm-links create` - Create
- `immybot-cli rmm-links get-by-id` - Get by id
- `immybot-cli rmm-links list` - List
- `immybot-cli rmm-links update-by-id` - Update by id

**roles** - Manage roles

- `immybot-cli roles create` - Create
- `immybot-cli roles delete-by-id` - Delete by id
- `immybot-cli roles get-by-id` - Get by id
- `immybot-cli roles get-permissions` - Get permissions
- `immybot-cli roles list` - List
- `immybot-cli roles update-by-id` - Update by id

**run-immy-service** - Manage run immy service

- `immybot-cli run-immy-service` - Create

**run-immy-service-new** - Manage run immy service new

- `immybot-cli run-immy-service-new` - Create

**schedules** - Manage schedules

- `immybot-cli schedules create` - Create
- `immybot-cli schedules create-bulk-cancel` - Create bulk cancel
- `immybot-cli schedules create-bulk-delete` - Create bulk delete
- `immybot-cli schedules create-bulk-run-now` - Create bulk run now
- `immybot-cli schedules delete-by-id` - Delete by id
- `immybot-cli schedules get-by-id` - Get by id
- `immybot-cli schedules get-running-ids` - Get running ids
- `immybot-cli schedules list` - List
- `immybot-cli schedules update-bulk-update-status` - Update bulk update status
- `immybot-cli schedules update-by-id` - Update by id

**scripts** - Manage scripts

- `immybot-cli scripts create-debug-cancel-by-cancellation-id` - Create debug cancel by cancellation id
- `immybot-cli scripts create-default-variables` - Create default variables
- `immybot-cli scripts create-does-have-param-block` - Create does have param block
- `immybot-cli scripts create-duplicate` - Create duplicate
- `immybot-cli scripts create-functions-syntax` - Execute a cloud script that returns the syntax for a specific command
- `immybot-cli scripts create-global` - Create global
- `immybot-cli scripts create-global-by-id` - Create global by id
- `immybot-cli scripts create-language-service-start` - Create language service start
- `immybot-cli scripts create-local` - Create local
- `immybot-cli scripts create-local-by-id` - Create local by id
- `immybot-cli scripts create-local-by-id-authorization` - Create local by id authorization
- `immybot-cli scripts create-local-by-id-migrate-local-to-global` - Create local by id migrate local to global
- `immybot-cli scripts create-run` - Create run
- `immybot-cli scripts create-run-adhoc-metascript` - Create run adhoc metascript
- `immybot-cli scripts create-set-preflight-enablement` - Create set preflight enablement
- `immybot-cli scripts create-syntax-check` - Create syntax check
- `immybot-cli scripts create-validate-param-block-parameters` - Create validate param block parameters
- `immybot-cli scripts delete-global-by-id` - Delete global by id
- `immybot-cli scripts delete-local-by-id` - Delete local by id
- `immybot-cli scripts get-disabled-preflight` - Get disabled preflight
- `immybot-cli scripts get-dx` - Get dx
- `immybot-cli scripts get-functions` - Execute a cloud script that returns results of Get-Command
- `immybot-cli scripts get-global` - Get global
- `immybot-cli scripts get-global-by-id` - Get global by id
- `immybot-cli scripts get-global-by-id-audit` - Get global by id audit
- `immybot-cli scripts get-global-by-id-references` - Get global by id references
- `immybot-cli scripts get-global-names` - Get global names
- `immybot-cli scripts get-language-service-by-terminal-id-language` - Get language service by terminal id language
- `immybot-cli scripts get-local` - Get local
- `immybot-cli scripts get-local-by-id` - Get local by id
- `immybot-cli scripts get-local-by-id-audit` - Get local by id audit
- `immybot-cli scripts get-local-by-id-authorization` - Get local by id authorization
- `immybot-cli scripts get-local-by-id-migrate-local-to-global-what-if` - Get local by id migrate local to global what if
- `immybot-cli scripts get-local-by-id-references` - Get local by id references
- `immybot-cli scripts get-local-names` - Get local names
- `immybot-cli scripts get-references-count` - Get references count
- `immybot-cli scripts get-search` - Get search

**smtp-configs** - Manage smtp configs

- `immybot-cli smtp-configs create` - Create
- `immybot-cli smtp-configs create-by-tenant-id` - Create by tenant id
- `immybot-cli smtp-configs create-send-test-email` - Create send test email
- `immybot-cli smtp-configs delete-by-tenant-id` - Delete by tenant id
- `immybot-cli smtp-configs get-by-tenant-id` - Get by tenant id
- `immybot-cli smtp-configs list` - List

**software** - Manage software

- `immybot-cli software create-global` - Create global
- `immybot-cli software create-global-analyze` - Create global analyze
- `immybot-cli software create-global-by-identifier-versions` - Create global by identifier versions
- `immybot-cli software create-global-fast-create` - Create global fast create
- `immybot-cli software create-global-upload` - Create global upload
- `immybot-cli software create-local` - Create local
- `immybot-cli software create-local-analyze` - Create local analyze
- `immybot-cli software create-local-by-identifier-authorization` - Create local by identifier authorization
- `immybot-cli software create-local-by-identifier-migrate-local-to-global` - Create local by identifier migrate local to global
- `immybot-cli software create-local-by-identifier-versions` - Create local by identifier versions
- `immybot-cli software create-local-fast-create` - Create local fast create
- `immybot-cli software create-local-upload` - Create local upload
- `immybot-cli software delete-global-by-identifier` - Delete global by identifier
- `immybot-cli software delete-global-by-identifier-versions-by-semantic-version` - Delete global by identifier versions by semantic version
- `immybot-cli software delete-local-by-identifier` - Delete local by identifier
- `immybot-cli software delete-local-by-identifier-versions-by-semantic-version` - Delete local by identifier versions by semantic version
- `immybot-cli software get-global` - Get global
- `immybot-cli software get-global-by-identifier` - Get global by identifier
- `immybot-cli software get-global-by-identifier-latest` - Get global by identifier latest
- `immybot-cli software get-global-by-identifier-versions` - Get global by identifier versions
- `immybot-cli software get-global-by-identifier-versions-by-semantic-version` - Get global by identifier versions by semantic version
- `immybot-cli software get-global-by-identifier-versions-by-semantic-version-request-download` - Get global by identifier versions by semantic version request download
- `immybot-cli software get-local` - Get local
- `immybot-cli software get-local-by-identifier` - Get local by identifier
- `immybot-cli software get-local-by-identifier-authorization` - Get local by identifier authorization
- `immybot-cli software get-local-by-identifier-latest` - Get local by identifier latest
- `immybot-cli software get-local-by-identifier-migrate-local-to-global-what-if` - Get local by identifier migrate local to global what if
- `immybot-cli software get-local-by-identifier-versions` - Get local by identifier versions
- `immybot-cli software get-local-by-identifier-versions-by-semantic-version` - Get local by identifier versions by semantic version
- `immybot-cli software get-local-by-identifier-versions-by-semantic-version-request-download` - Get local by identifier versions by semantic version request download
- `immybot-cli software update-global-by-identifier` - Update global by identifier
- `immybot-cli software update-global-by-identifier-versions-by-semantic-version` - Update global by identifier versions by semantic version
- `immybot-cli software update-local-by-identifier` - Update local by identifier
- `immybot-cli software update-local-by-identifier-versions-by-semantic-version` - Update local by identifier versions by semantic version

**syncs** - Manage syncs

- `immybot-cli syncs create-azure-user` - Create azure user
- `immybot-cli syncs create-expire-pending-sessions` - Create expire pending sessions
- `immybot-cli syncs create-trigger-user-affinity` - Create trigger user affinity

**system** - Manage system

- `immybot-cli system create-disable-immy-support-access` - Create disable immy support access
- `immybot-cli system create-enable-immy-support-access` - Create enable immy support access
- `immybot-cli system create-is-immy-support-access-granted` - Create is immy support access granted
- `immybot-cli system create-pull-update` - Create pull update
- `immybot-cli system create-request-form-support` - Create request form support
- `immybot-cli system create-request-session-support` - Create request session support
- `immybot-cli system create-restart-backend` - Create restart backend
- `immybot-cli system create-update-release-channel` - Create update release channel
- `immybot-cli system get-immy-support-access-grant-details` - Get immy support access grant details
- `immybot-cli system get-releases` - Get releases
- `immybot-cli system get-timezones` - Get timezones

**tags** - Manage tags

- `immybot-cli tags create` - Create
- `immybot-cli tags create-by-id` - Create by id
- `immybot-cli tags delete-by-id` - Delete by id
- `immybot-cli tags get-by-id` - Get by id
- `immybot-cli tags list` - List

**target-assignments** - Manage target assignments

- `immybot-cli target-assignments create` - Create
- `immybot-cli target-assignments create-change-request-by-change-request-id-v2` - Create change request by change request id v2
- `immybot-cli target-assignments create-change-request-v2` - Create change request v2
- `immybot-cli target-assignments create-duplicate` - Create duplicate
- `immybot-cli target-assignments create-duplicates` - Create duplicates
- `immybot-cli target-assignments create-global-by-id-notes` - Create global by id notes
- `immybot-cli target-assignments create-global-by-id-override` - Create global by id override
- `immybot-cli target-assignments create-global-create` - Create global create
- `immybot-cli target-assignments create-migrate-deployments-to-provider-links` - Create migrate deployments to provider links
- `immybot-cli target-assignments create-migrate-to-superseding-assignment` - Create migrate to superseding assignment
- `immybot-cli target-assignments create-migrate-to-superseding-assignment-what-if` - Create migrate to superseding assignment what if
- `immybot-cli target-assignments create-optional-approvals-by-id` - Create optional approvals by id
- `immybot-cli target-assignments create-persons-target-preview` - Create persons target preview
- `immybot-cli target-assignments create-recommended-approvals-update` - Create recommended approvals update
- `immybot-cli target-assignments create-target-preview` - Create target preview
- `immybot-cli target-assignments create-tenant-target-preview` - Create tenant target preview
- `immybot-cli target-assignments create-update-maintenance-item-order` - Create update maintenance item order
- `immybot-cli target-assignments create-visibility` - Create visibility
- `immybot-cli target-assignments delete-by-id` - Delete by id
- `immybot-cli target-assignments delete-global-by-id` - Delete global by id
- `immybot-cli target-assignments get-by-id` - Get by id
- `immybot-cli target-assignments get-change-request-by-change-request-id` - Get change request by change request id
- `immybot-cli target-assignments get-change-request-by-change-request-id-diff` - Get change request by change request id diff
- `immybot-cli target-assignments get-change-requests` - Get change requests
- `immybot-cli target-assignments get-global` - Get global
- `immybot-cli target-assignments get-global-by-id` - Get global by id
- `immybot-cli target-assignments get-global-by-id-type` - Get global by id type
- `immybot-cli target-assignments get-maintenance-item-orders` - Get maintenance item orders
- `immybot-cli target-assignments get-optional-approvals-computer-by-computer-id` - Get optional approvals computer by computer id
- `immybot-cli target-assignments get-recommended-approvals` - Get recommended approvals
- `immybot-cli target-assignments list` - List
- `immybot-cli target-assignments update-batch-update` - Update batch update
- `immybot-cli target-assignments update-by-id` - Update by id
- `immybot-cli target-assignments update-global-by-id` - Update global by id

**tenants** - Manage tenants

- `immybot-cli tenants create` - Create
- `immybot-cli tenants create-add-tags` - Create add tags
- `immybot-cli tenants create-bulk-create` - Create bulk create
- `immybot-cli tenants create-bulk-delete` - Create bulk delete
- `immybot-cli tenants create-bulk-merge` - Create bulk merge
- `immybot-cli tenants create-remove-parent` - Create remove parent
- `immybot-cli tenants create-remove-tags` - Create remove tags
- `immybot-cli tenants create-resolve-assignments-for-maintenance-item` - Create resolve assignments for maintenance item
- `immybot-cli tenants create-set-parent` - Create set parent
- `immybot-cli tenants create-update-azure-link` - Create update azure link
- `immybot-cli tenants get-by-id` - Get by id
- `immybot-cli tenants get-computer-counts` - Get computer counts
- `immybot-cli tenants get-excluded-from-cross-deployments` - Get excluded from cross deployments
- `immybot-cli tenants get-software-from-inventory-by-id` - Get software from inventory by id
- `immybot-cli tenants get-software-from-inventory-dx` - Get software from inventory dx
- `immybot-cli tenants get-software-from-inventory-export` - Streams the contents of the detected computer software table as a CSV file to the client
- `immybot-cli tenants list` - List
- `immybot-cli tenants update-activate-by-id` - Update activate by id
- `immybot-cli tenants update-by-id` - Update by id
- `immybot-cli tenants update-deactivate-by-id` - Update deactivate by id

**user-role-assignments** - Manage user role assignments

- `immybot-cli user-role-assignments create-category-resource-create` - Create category resource create
- `immybot-cli user-role-assignments create-msp-create` - Create msp create
- `immybot-cli user-role-assignments create-owner-create` - Create owner create
- `immybot-cli user-role-assignments create-specific-resource-create` - Create specific resource create
- `immybot-cli user-role-assignments create-specific-tenant-create` - Create specific tenant create
- `immybot-cli user-role-assignments create-tag-resource-create` - Create tag resource create
- `immybot-cli user-role-assignments create-tenant-tag-create` - Create tenant tag create
- `immybot-cli user-role-assignments create-user-tenant-create` - Create user tenant create
- `immybot-cli user-role-assignments delete-delete` - Delete delete
- `immybot-cli user-role-assignments get-users-by-user-id` - Get users by user id
- `immybot-cli user-role-assignments get-users-by-user-id-count` - Get users by user id count
- `immybot-cli user-role-assignments list` - List

**user_session** - Manage user session

- `immybot-cli user-session get-login` - Get login
- `immybot-cli user-session get-logout` - Get logout
- `immybot-cli user-session get-me` - Get me
- `immybot-cli user-session get-refresh` - Get refresh

**users** - Manage users

- `immybot-cli users create-bulk-create` - Create bulk create
- `immybot-cli users create-by-id` - Create by id
- `immybot-cli users create-invalidate-cache` - Create invalidate cache
- `immybot-cli users create-stop-impersonating` - Create stop impersonating
- `immybot-cli users create-submit-feedback` - Create submit feedback
- `immybot-cli users create-update-expiration` - Create update expiration
- `immybot-cli users delete-bulk-delete` - Delete bulk delete
- `immybot-cli users delete-by-id` - Delete by id
- `immybot-cli users get-by-id` - Get by id
- `immybot-cli users get-claims` - Get claims
- `immybot-cli users list` - List

**webhooks** - Manage webhooks

- `immybot-cli webhooks create-by-id` - Create by id
- `immybot-cli webhooks get-by-id` - Get by id


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
immybot-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match - fall back to `--help` or use a narrower query.

## Recipes

### Morning triage in one call

```bash
immybot-cli session-triage --since 24h --agent --select clusters.reason,clusters.action,clusters.computer_count
```

Returns only the distinct failure causes and how many machines each hit, which is the whole decision surface for opening tickets.

### CVE sweep across every client

```bash
immybot-cli version-spread "Google Chrome" --min-version 140 --agent
```

Ranks installed versions with a real semver comparator and lists the tenants and machines still below the floor.

### Explain a missed deployment

```bash
immybot-cli assignment-explain 4821 --agent
```

Shows every target assignment resolving onto that computer, which scope matched, and which rules were shadowed.

### Check reach before editing a shared script

```bash
immybot-cli script-blast-radius 312 --agent
```

Walks the script to its consuming tasks and packages and out to the computers those assignments resolve onto.

### Find silently stalled onboards

```bash
immybot-cli onboarding-stalled --older-than 3d --agent
```

Buckets the onboarding queue by age and shows whether an onboarding session was ever attempted and how it ended.

## Auth Setup

ImmyBot authenticates through Microsoft Entra ID rather than an ImmyBot-issued key. Register an app in Entra ID, create a client secret, then in ImmyBot go to Show More > People > New and paste the Enterprise Application's object ID into the AD External ID field, promoting that person to an admin user. The CLI then needs four values: IMMYBOT_SUBDOMAIN (your instance name without .immy.bot), IMMYBOT_TENANT_ID, IMMYBOT_CLIENT_ID, and IMMYBOT_CLIENT_SECRET. The client-credentials token is minted against login.microsoftonline.com and cached automatically; the API scope is derived from your instance URL, and IMMYBOT_OAUTH_SCOPE overrides it if your tenant exposes a different App ID URI.

Run `immybot-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

- **Pipeable** - JSON on stdout, errors on stderr
- **Filterable** - `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  immybot-cli access list --agent --select addons,backendRegAppId,canManageCrossTenantDeployments
  ```
- **Previewable** - `--dry-run` shows the request without sending
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Non-interactive** - never prompts, every input is a flag
- **Explicit confirmation** - `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Explicit retries** - use `--idempotent` only when an already-existing create should count as success, and use `--ignore-missing` only when a missing delete target should count as success

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set - piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Paths and state

Agents should treat the CLI's path resolver as part of the runtime contract:

- Use `--home <dir>` for one invocation, or set `IMMYBOT_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `IMMYBOT_CONFIG_DIR`, `IMMYBOT_DATA_DIR`, `IMMYBOT_STATE_DIR`, `IMMYBOT_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `IMMYBOT_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `immybot-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "immybot": {
        "command": "immybot-mcp",
        "env": {
          "IMMYBOT_HOME": "/srv/immybot"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `IMMYBOT_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `IMMYBOT_HOME`, or `doctor` will not find credentials left under the former root.

## Automatic learning

This CLI ships a self-capturing learning loop. The CLI does its own bookkeeping: every invocation is journaled locally, a failed flag followed by a corrected retry auto-derives a `flag_alias` candidate, and a `teach` on a query family without a playbook auto-synthesizes a `playbook_candidate` from the session's journal. Your job is judgment only: `recall` first, act on surfaced candidates, `teach` the final answer, `playbook amend` when you observe a correction. You never record failures by hand.

### Step 1: `recall` before any discovery

Before list/search/drill commands on a new user question, run:

```bash
immybot-cli recall "<user's question>" --agent
```

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
      "next_action": ["<trial command>", "immybot-cli learnings confirm 12"] }
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
       materially more, record the divergence via `immybot-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `immybot-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities - direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `immybot-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately:

```bash
immybot-cli teach --query "<user's question>" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
immybot-cli teach \
  --query "<user's question>" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
immybot-cli teach-playbook \
  --query "<user's question>" \
  --playbook-file ~/playbooks/<shape>.json \
  --notes-file ~/playbooks/<shape>-notes.md
```

Playbook files are JSON with `steps`, `entity_slots`, `expected_tool_calls`. Notes files are markdown carrying the gotchas verbatim. File-free callers (MCP-only agents) pass the same content inline: `--playbook-json` and `--playbook-notes` on the integrated `teach` form, `--playbook-json` and `--notes` on `teach-playbook`. On the integrated `teach` form, the playbook flags are optional - omit them entirely for a resource-only teach. On the standalone `teach-playbook` form, at least one of the playbook and notes flags must be set; both empty is rejected. Playbooks are keyed on the structural query family (entities stripped) so a recipe taught from one entity-shaped query applies to every other query of the same shape, with `slots_resolved` binding the live query's canonical at recall time.

When you DO find a playbook on a future recall, treat it as ground truth: replay the steps with `slots_resolved` substitutions, skip the discovery that the choreography already documents, and read `notes` before any step.

### Step 6: `playbook amend &` when your debug response identifies a correction

If your debug-protocol response identifies a concrete correction the notes or playbook should know - a workaround, an undocumented endpoint shape, a stale field name, observed schema drift, an empty-payload fallback - fire `playbook amend` BEFORE emitting your user-facing response. Same fire-and-forget posture as `teach`.

```bash
immybot-cli playbook amend \
  --query "<exact recall query string>" \
  --add-note "<your concrete correction>"
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

`immybot-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `IMMYBOT_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
immybot-cli feedback "the --since flag is inclusive but docs say exclusive"
immybot-cli feedback --stdin < notes.txt
immybot-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `IMMYBOT_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `IMMYBOT_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
immybot-cli profile save briefing --json
immybot-cli --profile briefing access list
immybot-cli profile list --json
immybot-cli profile show briefing
immybot-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `immybot-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

Install the MCP binary from this CLI's published public-library entry or pre-built release, then register it:

```bash
claude mcp add immybot-mcp -- immybot-mcp
```

Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which immybot-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   immybot-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `immybot-cli <command> --help`.
