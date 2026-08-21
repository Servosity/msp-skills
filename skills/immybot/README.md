# ImmyBot CLI

**Every ImmyBot endpoint typed, plus a local SQLite mirror that answers the cross-tenant questions the web UI cannot.**

ImmyBot's API is large and entirely per-tenant, so the questions MSPs actually ask span calls the web UI never joins. This CLI types the whole surface, mirrors it into local SQLite with full-text search, and adds commands built on that mirror: session-triage collapses a night of failures into distinct root causes, version-spread ranks one software title across every tenant with a real semver comparator, and assignment-explain shows which deployment rule actually won on a given machine.

Learn more at [ImmyBot](https://www.immy.bot).

Created by [@geekbrownbear](https://github.com/geekbrownbear) (Abhi Saini).

## Install

The recommended path installs both the `immybot-pp-cli` binary and the `pp-immybot` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install immybot
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install immybot --cli-only
```

For skill only — installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install immybot --skill-only
```

To constrain the skill install to one or more specific agents (repeatable — agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install immybot --agent claude-code
npx -y @mvanhorn/printing-press-library install immybot --agent claude-code --agent codex
```

### Without Node

The generated install path is category-agnostic until this CLI is published. If `npx` is not available before publish, install Node or use the category-specific Go fallback from the public-library entry after publish.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/immybot-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install immybot --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-immybot --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-immybot --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install immybot --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle — Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/immybot-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `IMMYBOT_TENANT_ID` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


Install the MCP binary from this CLI's published public-library entry or pre-built release.

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "immybot": {
      "command": "immybot-pp-mcp",
      "env": {
        "IMMYBOT_SUBDOMAIN": "<subdomain>",
        "IMMYBOT_TENANT_ID": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

ImmyBot authenticates through Microsoft Entra ID rather than an ImmyBot-issued key. Register an app in Entra ID, create a client secret, then in ImmyBot go to Show More > People > New and paste the Enterprise Application's object ID into the AD External ID field, promoting that person to an admin user. The CLI then needs four values: IMMYBOT_SUBDOMAIN (your instance name without .immy.bot), IMMYBOT_TENANT_ID, IMMYBOT_CLIENT_ID, and IMMYBOT_CLIENT_SECRET. The client-credentials token is minted against login.microsoftonline.com and cached automatically; the API scope is derived from your instance URL, and IMMYBOT_OAUTH_SCOPE overrides it if your tenant exposes a different App ID URI.

## Quick Start

```bash
# Confirm the instance subdomain and Entra credentials resolve before anything else.
immybot-pp-cli doctor

# Build the local mirror the cross-tenant commands read from.
immybot-pp-cli sync --resources tenants,computers

# Confirm the mirror has real rows.
immybot-pp-cli computers list --page-size 20

# Collapse the last maintenance window into distinct root causes.
immybot-pp-cli session-triage --since 24h

# Answer the recurring question: which tenants are still behind on a title.
immybot-pp-cli version-spread "Google Chrome" --min-version 140

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Local state that compounds
- **`session-triage`** — Group last night's failed maintenance actions by root cause instead of reading the same error on forty machines.

  _Reach for this first after any maintenance window: it turns N red machines into the handful of distinct problems actually worth a ticket._

  ```bash
  immybot-pp-cli session-triage --since 24h --agent
  ```
- **`version-spread`** — Semver-ordered distribution of one software title across every tenant, flagging everything below a floor.

  _This is the CVE-response command: one call answers which clients are still exposed on a given title._

  ```bash
  immybot-pp-cli version-spread "Google Chrome" --min-version 140 --agent
  ```
- **`fleet-diff`** — What actually changed between two syncs: computers added or removed, software versions moved, assignments modified.

  _Use this to answer "what changed since last night" without diffing two exports by hand._

  ```bash
  immybot-pp-cli fleet-diff --since 24h --agent
  ```
- **`onboarding-stalled`** — Computers stuck waiting to onboard, bucketed by age and annotated with whether onboarding was ever attempted.

  _Surfaces machines that silently never finished onboarding, which is the failure mode clients notice first._

  ```bash
  immybot-pp-cli onboarding-stalled --older-than 3d --agent
  ```

### Deployment resolution
- **`assignment-explain`** — Show every target assignment that resolves onto one computer, which scope matched, and which rules are shadowed.

  _Use this for any "why didn't this machine get X" question; it answers what a computer receives and why, which no single endpoint does._

  ```bash
  immybot-pp-cli assignment-explain 4821 --agent
  ```
- **`script-blast-radius`** — Every maintenance task, software package, and computer that a script reaches before you edit it.

  _Run this before editing any shared script; it is the only way to see downstream reach across tenants._

  ```bash
  immybot-pp-cli script-blast-radius 312 --agent
  ```

### Integration hygiene
- **`psa-reconcile`** — Diff the ImmyBot roster against a linked PSA or RMM asset roster to find unlinked computers and orphaned assets.

  _Run after each week of onboards and decommissions; mapping gaps otherwise surface as a wrong invoice or a machine that stopped getting maintenance._

  ```bash
  immybot-pp-cli psa-reconcile --provider 7 --agent
  ```

## Recipes

### Morning triage in one call

```bash
immybot-pp-cli session-triage --since 24h --agent --select clusters.reason,clusters.action,clusters.computer_count
```

Returns only the distinct failure causes and how many machines each hit, which is the whole decision surface for opening tickets.

### CVE sweep across every client

```bash
immybot-pp-cli version-spread "Google Chrome" --min-version 140 --agent
```

Ranks installed versions with a real semver comparator and lists the tenants and machines still below the floor.

### Explain a missed deployment

```bash
immybot-pp-cli assignment-explain 4821 --agent
```

Shows every target assignment resolving onto that computer, which scope matched, and which rules were shadowed.

### Check reach before editing a shared script

```bash
immybot-pp-cli script-blast-radius 312 --agent
```

Walks the script to its consuming tasks and packages and out to the computers those assignments resolve onto.

### Find silently stalled onboards

```bash
immybot-pp-cli onboarding-stalled --older-than 3d --agent
```

Buckets the onboarding queue by age and shows whether an onboarding session was ever attempted and how it ended.

## Usage

Run `immybot-pp-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `IMMYBOT_CONFIG_DIR`, `IMMYBOT_DATA_DIR`, `IMMYBOT_STATE_DIR`, or `IMMYBOT_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `IMMYBOT_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export IMMYBOT_HOME=/srv/immybot
immybot-pp-cli doctor
```

Under `IMMYBOT_HOME=/srv/immybot`, the four dirs resolve to `/srv/immybot/config`, `/srv/immybot/data`, `/srv/immybot/state`, and `/srv/immybot/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "immybot": {
      "command": "immybot-pp-mcp",
      "env": {
        "IMMYBOT_HOME": "/srv/immybot"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `IMMYBOT_DATA_DIR` overrides an explicit `--home` for that kind. Use `IMMYBOT_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `IMMYBOT_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `immybot-pp-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### access

Manage access

- **`immybot-pp-cli access create-delete-azure-tenant-auth-details`** - Create delete azure tenant auth details
- **`immybot-pp-cli access create-request`** - Create request
- **`immybot-pp-cli access create-update-azure-tenant-auth-details`** - Create update azure tenant auth details
- **`immybot-pp-cli access get-get-azure-tenant-auth-details-by-azure-tenant-principal-id`** - Get get azure tenant auth details by azure tenant principal id
- **`immybot-pp-cli access get-get-ip-addresses`** - Get get ip addresses
- **`immybot-pp-cli access get-me-permissions-by-permission-type-tenants`** - Get me permissions by permission type tenants
- **`immybot-pp-cli access list`** - List

### application-locks

Manage application locks

- **`immybot-pp-cli application-locks create-request-cancellation`** - Create request cancellation
- **`immybot-pp-cli application-locks get-realtime-event-stream`** - Get realtime event stream
- **`immybot-pp-cli application-locks list`** - List

### application-logs

Manage application logs

- **`immybot-pp-cli application-logs create-source-context`** - Create source context
- **`immybot-pp-cli application-logs create-source-context-clear`** - Create source context clear
- **`immybot-pp-cli application-logs create-source-context-clear-all`** - Create source context clear all
- **`immybot-pp-cli application-logs create-streaming`** - Create streaming
- **`immybot-pp-cli application-logs get-source-contexts`** - Get source contexts

### audits

Manage audits

- **`immybot-pp-cli audits get-global-dx`** - Get global dx
- **`immybot-pp-cli audits get-local-dx`** - Get local dx

### azure

Manage azure

- **`immybot-pp-cli azure create-disambiguate-tenant-type`** - Create disambiguate tenant type
- **`immybot-pp-cli azure create-preconsent-customer-tenants`** - Create preconsent customer tenants
- **`immybot-pp-cli azure create-sync-details-from-tenants`** - Create sync details from tenants
- **`immybot-pp-cli azure create-sync-users-from-tenants`** - Create sync users from tenants
- **`immybot-pp-cli azure create-tenant-consented`** - Create tenant consented
- **`immybot-pp-cli azure get-app-registration-options`** - Get app registration options
- **`immybot-pp-cli azure get-partner-tenant-customers-by-partner-principal-id`** - Get partner tenant customers by partner principal id
- **`immybot-pp-cli azure get-partner-tenant-infos`** - Get partner tenant infos

### azure-errors

Manage azure errors

- **`immybot-pp-cli azure-errors get-dx`** - Get dx
- **`immybot-pp-cli azure-errors get-for-tenant-by-tenant-principal-id-dx`** - Get for tenant by tenant principal id dx

### billing

Manage billing

- **`immybot-pp-cli billing create-cancel-subscription`** - Create cancel subscription
- **`immybot-pp-cli billing create-information`** - Create information
- **`immybot-pp-cli billing create-reactivate-subscription`** - Create reactivate subscription
- **`immybot-pp-cli billing create-update-addon`** - Create update addon
- **`immybot-pp-cli billing create-update-subscription`** - Create update subscription
- **`immybot-pp-cli billing get-credit-cards`** - Get credit cards
- **`immybot-pp-cli billing get-download-invoice`** - Get download invoice
- **`immybot-pp-cli billing get-feature-usage-counts`** - Get feature usage counts
- **`immybot-pp-cli billing get-information`** - Get information
- **`immybot-pp-cli billing get-platform-details`** - Get platform details
- **`immybot-pp-cli billing get-product-catalog`** - Get product catalog
- **`immybot-pp-cli billing get-product-catalog-items`** - Get product catalog items
- **`immybot-pp-cli billing get-subscription-details`** - Get subscription details

### brandings

Manage brandings

- **`immybot-pp-cli brandings create`** - Create
- **`immybot-pp-cli brandings create-global-default-by-id`** - Create global default by id
- **`immybot-pp-cli brandings create-send-test-email`** - Create send test email
- **`immybot-pp-cli brandings create-validate-time-format-by-time-format`** - Create validate time format by time format
- **`immybot-pp-cli brandings delete-by-id`** - Delete by id
- **`immybot-pp-cli brandings get-by-id`** - Get by id
- **`immybot-pp-cli brandings get-support`** - Fetches support related branding changes to be used in the Support Sidebar, Session Support Request, or other Support related UI.
These branding changes can be specified by Dynamic Providers implementing 'ISupportsSupportTicketDetailOverride'
- **`immybot-pp-cli brandings list`** - List
- **`immybot-pp-cli brandings update-by-id`** - Update by id

### change-requests

Manage change requests

- **`immybot-pp-cli change-requests delete-by-id`** - Delete by id
- **`immybot-pp-cli change-requests get-dx`** - Get dx
- **`immybot-pp-cli change-requests get-open-count`** - Get open count

### chocolatey

Manage chocolatey

- **`immybot-pp-cli chocolatey get-find-packages-by-id`** - Get find packages by id
- **`immybot-pp-cli chocolatey get-search`** - Get search

### computers

Manage computers

- **`immybot-pp-cli computers create-add-tags`** - Create add tags
- **`immybot-pp-cli computers create-bulk-delete`** - Create bulk delete
- **`immybot-pp-cli computers create-change-tenant`** - Create change tenant
- **`immybot-pp-cli computers create-remove-tags`** - Create remove tags
- **`immybot-pp-cli computers create-restore`** - Create restore
- **`immybot-pp-cli computers create-set-excluded-from-user-affinity`** - Create set excluded from user affinity
- **`immybot-pp-cli computers create-skip-onboarding`** - Create skip onboarding
- **`immybot-pp-cli computers get-agent-status`** - Get agent status
- **`immybot-pp-cli computers get-by-id`** - Get by id
- **`immybot-pp-cli computers get-dx`** - Get dx
- **`immybot-pp-cli computers get-export`** - Get export
- **`immybot-pp-cli computers get-inventory`** - Get inventory
- **`immybot-pp-cli computers get-inventory-export`** - Get inventory export
- **`immybot-pp-cli computers get-inventory-software-search-by-name`** - Get inventory software search by name
- **`immybot-pp-cli computers get-inventory-software-search-by-upgrade-code`** - Get inventory software search by upgrade code
- **`immybot-pp-cli computers get-my`** - Get my
- **`immybot-pp-cli computers get-onboarding`** - Get onboarding
- **`immybot-pp-cli computers get-paged`** - TODO: Move this to V2 api routes or make the existing GetAll rely on this
- **`immybot-pp-cli computers get-user-affinities`** - Get user affinities
- **`immybot-pp-cli computers get-user-affinities-export`** - Get user affinities export
- **`immybot-pp-cli computers list`** - List
- **`immybot-pp-cli computers update-by-id`** - Update by id

### dynamic-provider-types

Manage dynamic provider types

- **`immybot-pp-cli dynamic-provider-types create-global`** - Create global
- **`immybot-pp-cli dynamic-provider-types create-global-by-id`** - Create global by id
- **`immybot-pp-cli dynamic-provider-types create-global-by-id-reload`** - Create global by id reload
- **`immybot-pp-cli dynamic-provider-types create-local`** - Create local
- **`immybot-pp-cli dynamic-provider-types create-local-by-id`** - Create local by id
- **`immybot-pp-cli dynamic-provider-types create-local-by-id-reload`** - Create local by id reload
- **`immybot-pp-cli dynamic-provider-types create-reload`** - Create reload
- **`immybot-pp-cli dynamic-provider-types create-test-environment-by-terminal-id`** - Create test environment by terminal id
- **`immybot-pp-cli dynamic-provider-types create-test-environment-by-terminal-id-bind-configuration-form`** - Create test environment by terminal id bind configuration form
- **`immybot-pp-cli dynamic-provider-types create-test-environment-by-terminal-id-execute-method-by-method`** - Create test environment by terminal id execute method by method
- **`immybot-pp-cli dynamic-provider-types delete-global-by-id`** - Delete global by id
- **`immybot-pp-cli dynamic-provider-types delete-local-by-id`** - Delete local by id
- **`immybot-pp-cli dynamic-provider-types delete-test-environment-by-terminal-id`** - Delete test environment by terminal id
- **`immybot-pp-cli dynamic-provider-types get-global-by-id`** - Get global by id
- **`immybot-pp-cli dynamic-provider-types get-local-by-id`** - Get local by id
- **`immybot-pp-cli dynamic-provider-types list`** - List

### effective-permissions

Manage effective permissions

- **`immybot-pp-cli effective-permissions create-groups-by-group-id-evaluate-all-assignments`** - Returns all role assignments for a group grouped by permission without evaluation context.
Shows assignment overview without determining effective allow/deny.
- **`immybot-pp-cli effective-permissions create-groups-by-group-id-evaluate-resource`** - Evaluates permissions for a group against a specific resource.
Determines effective allow/deny for each permission within that resource context.
- **`immybot-pp-cli effective-permissions create-groups-by-group-id-evaluate-tenant`** - Evaluates permissions for a group against a specific tenant.
Determines effective allow/deny for each permission within that tenant context.
- **`immybot-pp-cli effective-permissions create-users-by-user-id-evaluate-all-assignments`** - Returns all role assignments for a user grouped by permission without evaluation context.
Shows assignment overview without determining effective allow/deny.
- **`immybot-pp-cli effective-permissions create-users-by-user-id-evaluate-resource`** - Evaluates permissions for a user against a specific resource.
Determines effective allow/deny for each permission within that resource context.
- **`immybot-pp-cli effective-permissions create-users-by-user-id-evaluate-tenant`** - Evaluates permissions for a user against a specific tenant.
Determines effective allow/deny for each permission within that tenant context.

### ephemeral-session

Manage ephemeral session

- **`immybot-pp-cli ephemeral-session get-by-agent-instance-id-by-provider-agent-id`** - Get by agent instance id by provider agent id
- **`immybot-pp-cli ephemeral-session get-development-latest-ephemeral-binary`** - Get development latest ephemeral binary
- **`immybot-pp-cli ephemeral-session get-development-latest-ephemeral-binary-v2`** - Get development latest ephemeral binary v2

### getting-started

Manage getting started

- **`immybot-pp-cli getting-started create-checklist-complete`** - Create checklist complete
- **`immybot-pp-cli getting-started create-checklist-reset`** - Create checklist reset
- **`immybot-pp-cli getting-started get-checklist`** - Get checklist

### groups

Manage groups

- **`immybot-pp-cli groups create`** - Create
- **`immybot-pp-cli groups delete-by-id`** - Delete by id
- **`immybot-pp-cli groups get-by-id`** - Get by id
- **`immybot-pp-cli groups list`** - List
- **`immybot-pp-cli groups update-by-id`** - Update by id

### immy-agent-metadata

Manage immy agent metadata

- **`immybot-pp-cli immy-agent-metadata`** - Get agent hash

### installer

Manage installer

- **`immybot-pp-cli installer`** - Create agent rekey request

### inventory-tasks

Manage inventory tasks

- **`immybot-pp-cli inventory-tasks create-local`** - Create local
- **`immybot-pp-cli inventory-tasks create-local-by-id`** - Create local by id
- **`immybot-pp-cli inventory-tasks create-local-by-id-scripts`** - Create local by id scripts
- **`immybot-pp-cli inventory-tasks delete-local-by-id`** - Delete local by id
- **`immybot-pp-cli inventory-tasks delete-local-by-task-id-scripts-by-inventory-key`** - Delete local by task id scripts by inventory key
- **`immybot-pp-cli inventory-tasks list`** - List

### licenses

Manage licenses

- **`immybot-pp-cli licenses create`** - Create
- **`immybot-pp-cli licenses create-upload`** - Create upload
- **`immybot-pp-cli licenses delete-by-id`** - Delete by id
- **`immybot-pp-cli licenses get-by-id`** - Get by id
- **`immybot-pp-cli licenses get-dx`** - Get dx
- **`immybot-pp-cli licenses list`** - List
- **`immybot-pp-cli licenses update-by-id`** - Update by id

### maintenance-actions

Manage maintenance actions

- **`immybot-pp-cli maintenance-actions create-latest-action-for-computers`** - Create latest action for computers
- **`immybot-pp-cli maintenance-actions create-latest-action-for-tenants`** - Create latest action for tenants
- **`immybot-pp-cli maintenance-actions get-computer-by-computer-id-needs-attention`** - Get computer by computer id needs attention
- **`immybot-pp-cli maintenance-actions get-dx`** - Get dx
- **`immybot-pp-cli maintenance-actions get-dx-for-computer-by-computer-id`** - Get dx for computer by computer id
- **`immybot-pp-cli maintenance-actions get-latest-for-computer-by-computer-id`** - Get latest for computer by computer id
- **`immybot-pp-cli maintenance-actions get-latest-for-tenant-by-tenant-id`** - Get latest for tenant by tenant id
- **`immybot-pp-cli maintenance-actions get-latest-non-compliant-actions-for-tenant-by-tenant-id`** - Get latest non compliant actions for tenant by tenant id
- **`immybot-pp-cli maintenance-actions get-maintenance-item`** - Get maintenance item
- **`immybot-pp-cli maintenance-actions get-version`** - Get version

### maintenance-emails

Manage maintenance emails


### maintenance-sessions

Manage maintenance sessions

- **`immybot-pp-cli maintenance-sessions create-cancel`** - Create cancel
- **`immybot-pp-cli maintenance-sessions create-cancel-all`** - Create cancel all
- **`immybot-pp-cli maintenance-sessions create-rerun-v2`** - Create rerun v2
- **`immybot-pp-cli maintenance-sessions get-by-session-id`** - Get by session id
- **`immybot-pp-cli maintenance-sessions get-cancel-for-schedule-by-schedule-id`** - Get cancel for schedule by schedule id
- **`immybot-pp-cli maintenance-sessions get-dx`** - Get dx
- **`immybot-pp-cli maintenance-sessions get-status-counts`** - Get status counts

### maintenance-tasks

Manage maintenance tasks

- **`immybot-pp-cli maintenance-tasks create-duplicate`** - Create duplicate
- **`immybot-pp-cli maintenance-tasks create-global`** - Create global
- **`immybot-pp-cli maintenance-tasks create-global-by-id`** - Create global by id
- **`immybot-pp-cli maintenance-tasks create-global-by-id-param-block-from-parameters`** - Create global by id param block from parameters
- **`immybot-pp-cli maintenance-tasks create-local`** - Create local
- **`immybot-pp-cli maintenance-tasks create-local-by-id`** - Create local by id
- **`immybot-pp-cli maintenance-tasks create-local-by-id-migrate-local-to-global`** - Create local by id migrate local to global
- **`immybot-pp-cli maintenance-tasks create-local-by-id-param-block-from-parameters`** - Create local by id param block from parameters
- **`immybot-pp-cli maintenance-tasks create-validate-param-block-parameters`** - Create validate param block parameters
- **`immybot-pp-cli maintenance-tasks delete-global-by-id`** - Delete global by id
- **`immybot-pp-cli maintenance-tasks delete-local-by-id`** - Delete local by id
- **`immybot-pp-cli maintenance-tasks get-global`** - Get global
- **`immybot-pp-cli maintenance-tasks get-global-by-id`** - Get global by id
- **`immybot-pp-cli maintenance-tasks get-local`** - Get local
- **`immybot-pp-cli maintenance-tasks get-local-by-id`** - Get local by id
- **`immybot-pp-cli maintenance-tasks get-local-by-id-migrate-local-to-global-what-if`** - Get local by id migrate local to global what if
- **`immybot-pp-cli maintenance-tasks get-reference-count`** - Get reference count
- **`immybot-pp-cli maintenance-tasks get-search`** - Get search

### me

Manage me

- **`immybot-pp-cli me`** - Gets all role assignments and groups for the current user

### media

Manage media

- **`immybot-pp-cli media create-global-by-id`** - Create global by id
- **`immybot-pp-cli media create-global-upload`** - Create global upload
- **`immybot-pp-cli media create-local-by-id`** - Create local by id
- **`immybot-pp-cli media create-local-by-id-authorization`** - Create local by id authorization
- **`immybot-pp-cli media create-local-upload`** - Create local upload
- **`immybot-pp-cli media create-request-file-download-url`** - Create request file download url
- **`immybot-pp-cli media create-support-upload`** - Create support upload
- **`immybot-pp-cli media delete-global-by-id`** - Delete global by id
- **`immybot-pp-cli media delete-local-by-id`** - Delete local by id
- **`immybot-pp-cli media get-global`** - Get global
- **`immybot-pp-cli media get-global-by-id`** - Get global by id
- **`immybot-pp-cli media get-global-by-id-download-url`** - Get global by id download url
- **`immybot-pp-cli media get-local`** - Get local
- **`immybot-pp-cli media get-local-by-id`** - Get local by id
- **`immybot-pp-cli media get-local-by-id-authorization`** - Get local by id authorization
- **`immybot-pp-cli media get-local-by-id-download-url`** - Get local by id download url
- **`immybot-pp-cli media get-search`** - Get search

### metrics

Manage metrics

- **`immybot-pp-cli metrics create-circuit-breakers-isolate`** - Create circuit breakers isolate
- **`immybot-pp-cli metrics create-circuit-breakers-reset`** - Create circuit breakers reset
- **`immybot-pp-cli metrics get-circuit-breakers`** - Get circuit breakers
- **`immybot-pp-cli metrics get-provider-links`** - Get provider links
- **`immybot-pp-cli metrics get-provider-links-by-provider-link-id-rate-limit-statistics`** - Returns the current rate limiter statistics for a provider link.
200: stats available. 204: provider not initialized. 404: provider does not support rate limiting or link not found.

### notifications

Manage notifications

- **`immybot-pp-cli notifications create-acknowledge`** - Create acknowledge
- **`immybot-pp-cli notifications get-dx`** - Get dx
- **`immybot-pp-cli notifications get-unacknowledged`** - Get unacknowledged
- **`immybot-pp-cli notifications list`** - List

### oauth

Manage oauth

- **`immybot-pp-cli oauth create-access-tokens-by-id-refresh`** - Create access tokens by id refresh
- **`immybot-pp-cli oauth create-begin-auth-code-flow`** - Create begin auth code flow
- **`immybot-pp-cli oauth create-fail-auth-code-flow`** - Create fail auth code flow
- **`immybot-pp-cli oauth create-finish-auth-code-flow`** - Create finish auth code flow
- **`immybot-pp-cli oauth delete-access-tokens-by-id`** - Delete access tokens by id
- **`immybot-pp-cli oauth get-access-tokens`** - Get access tokens
- **`immybot-pp-cli oauth get-access-tokens-by-id-by-access-token-id`** - Get access tokens by id by access token id

### persons

Manage persons

- **`immybot-pp-cli persons create`** - Create
- **`immybot-pp-cli persons create-add-tags`** - Create add tags
- **`immybot-pp-cli persons create-remove-tags`** - Create remove tags
- **`immybot-pp-cli persons delete-by-id`** - Delete by id
- **`immybot-pp-cli persons get-by-id`** - Get by id
- **`immybot-pp-cli persons get-dx`** - Get dx
- **`immybot-pp-cli persons get-requesting-access`** - Get requesting access
- **`immybot-pp-cli persons list`** - List
- **`immybot-pp-cli persons update-by-id`** - Update by id

### plugins

Manage plugins

- **`immybot-pp-cli plugins create-api-v1-by-provider-link-id-by-catch-all`** - Create api v1 by provider link id by catch all
- **`immybot-pp-cli plugins get-api-v1-by-provider-link-id-by-catch-all`** - Get api v1 by provider link id by catch all
- **`immybot-pp-cli plugins get-api-v1-by-provider-link-id-by-catch-all-v2`** - Get api v1 by provider link id by catch all v2

### preferences

Manage preferences

- **`immybot-pp-cli preferences get-tenants-by-tenant-id`** - Get tenants by tenant id
- **`immybot-pp-cli preferences list`** - List
- **`immybot-pp-cli preferences update-application`** - Update application
- **`immybot-pp-cli preferences update-my`** - Update my
- **`immybot-pp-cli preferences update-tenants-by-tenant-id`** - Update tenants by tenant id

### provider-agents

Manage provider agents

- **`immybot-pp-cli provider-agents create-bulk-delete-pending`** - Create bulk delete pending
- **`immybot-pp-cli provider-agents create-identify`** - Identify agents that are marked with  requiring manual identification
- **`immybot-pp-cli provider-agents create-resolve-failure-by-failure-id`** - Create resolve failure by failure id
- **`immybot-pp-cli provider-agents create-resolve-failures`** - Create resolve failures
- **`immybot-pp-cli provider-agents get-pending`** - Get pending
- **`immybot-pp-cli provider-agents get-pending-counts`** - Get pending counts

### provider-links

Manage provider links

- **`immybot-pp-cli provider-links create`** - Create
- **`immybot-pp-cli provider-links create-create-with-external-provider-reference`** - Create create with external provider reference
- **`immybot-pp-cli provider-links create-verify-with-external-provider-reference`** - Create verify with external provider reference
- **`immybot-pp-cli provider-links delete-by-id`** - Delete by id
- **`immybot-pp-cli provider-links get-by-id`** - Get by id
- **`immybot-pp-cli provider-links list`** - List
- **`immybot-pp-cli provider-links update-by-id`** - Update by id

### provider-types

Manage provider types

- **`immybot-pp-cli provider-types get-client-group-types-by-client-group-type-id-client-groups`** - Get client group types by client group type id client groups
- **`immybot-pp-cli provider-types get-device-group-types-by-device-group-type-id-device-groups`** - Get device group types by device group type id device groups
- **`immybot-pp-cli provider-types get-form-dropdown-options-by-key`** - Get form dropdown options by key
- **`immybot-pp-cli provider-types list`** - List

### rmm-links

Manage rmm links

- **`immybot-pp-cli rmm-links create`** - Create
- **`immybot-pp-cli rmm-links get-by-id`** - Get by id
- **`immybot-pp-cli rmm-links list`** - List
- **`immybot-pp-cli rmm-links update-by-id`** - Update by id

### roles

Manage roles

- **`immybot-pp-cli roles create`** - Create
- **`immybot-pp-cli roles delete-by-id`** - Delete by id
- **`immybot-pp-cli roles get-by-id`** - Get by id
- **`immybot-pp-cli roles get-permissions`** - Get permissions
- **`immybot-pp-cli roles list`** - List
- **`immybot-pp-cli roles update-by-id`** - Update by id

### run-immy-service

Manage run immy service

- **`immybot-pp-cli run-immy-service`** - Create

### run-immy-service-new

Manage run immy service new

- **`immybot-pp-cli run-immy-service-new`** - Create

### schedules

Manage schedules

- **`immybot-pp-cli schedules create`** - Create
- **`immybot-pp-cli schedules create-bulk-cancel`** - Create bulk cancel
- **`immybot-pp-cli schedules create-bulk-delete`** - Create bulk delete
- **`immybot-pp-cli schedules create-bulk-run-now`** - Create bulk run now
- **`immybot-pp-cli schedules delete-by-id`** - Delete by id
- **`immybot-pp-cli schedules get-by-id`** - Get by id
- **`immybot-pp-cli schedules get-running-ids`** - Get running ids
- **`immybot-pp-cli schedules list`** - List
- **`immybot-pp-cli schedules update-bulk-update-status`** - Update bulk update status
- **`immybot-pp-cli schedules update-by-id`** - Update by id

### scripts

Manage scripts

- **`immybot-pp-cli scripts create-debug-cancel-by-cancellation-id`** - Create debug cancel by cancellation id
- **`immybot-pp-cli scripts create-default-variables`** - Create default variables
- **`immybot-pp-cli scripts create-does-have-param-block`** - Create does have param block
- **`immybot-pp-cli scripts create-duplicate`** - Create duplicate
- **`immybot-pp-cli scripts create-functions-syntax`** - Execute a cloud script that returns the syntax for a specific command
- **`immybot-pp-cli scripts create-global`** - Create global
- **`immybot-pp-cli scripts create-global-by-id`** - Create global by id
- **`immybot-pp-cli scripts create-language-service-start`** - Create language service start
- **`immybot-pp-cli scripts create-local`** - Create local
- **`immybot-pp-cli scripts create-local-by-id`** - Create local by id
- **`immybot-pp-cli scripts create-local-by-id-authorization`** - Create local by id authorization
- **`immybot-pp-cli scripts create-local-by-id-migrate-local-to-global`** - Create local by id migrate local to global
- **`immybot-pp-cli scripts create-run`** - Create run
- **`immybot-pp-cli scripts create-run-adhoc-metascript`** - Create run adhoc metascript
- **`immybot-pp-cli scripts create-set-preflight-enablement`** - Create set preflight enablement
- **`immybot-pp-cli scripts create-syntax-check`** - Create syntax check
- **`immybot-pp-cli scripts create-validate-param-block-parameters`** - Create validate param block parameters
- **`immybot-pp-cli scripts delete-global-by-id`** - Delete global by id
- **`immybot-pp-cli scripts delete-local-by-id`** - Delete local by id
- **`immybot-pp-cli scripts get-disabled-preflight`** - Get disabled preflight
- **`immybot-pp-cli scripts get-dx`** - Get dx
- **`immybot-pp-cli scripts get-functions`** - Execute a cloud script that returns results of Get-Command
- **`immybot-pp-cli scripts get-global`** - Get global
- **`immybot-pp-cli scripts get-global-by-id`** - Get global by id
- **`immybot-pp-cli scripts get-global-by-id-audit`** - Get global by id audit
- **`immybot-pp-cli scripts get-global-by-id-references`** - Get global by id references
- **`immybot-pp-cli scripts get-global-names`** - Get global names
- **`immybot-pp-cli scripts get-language-service-by-terminal-id-language`** - Get language service by terminal id language
- **`immybot-pp-cli scripts get-local`** - Get local
- **`immybot-pp-cli scripts get-local-by-id`** - Get local by id
- **`immybot-pp-cli scripts get-local-by-id-audit`** - Get local by id audit
- **`immybot-pp-cli scripts get-local-by-id-authorization`** - Get local by id authorization
- **`immybot-pp-cli scripts get-local-by-id-migrate-local-to-global-what-if`** - Get local by id migrate local to global what if
- **`immybot-pp-cli scripts get-local-by-id-references`** - Get local by id references
- **`immybot-pp-cli scripts get-local-names`** - Get local names
- **`immybot-pp-cli scripts get-references-count`** - Get references count
- **`immybot-pp-cli scripts get-search`** - Get search

### smtp-configs

Manage smtp configs

- **`immybot-pp-cli smtp-configs create`** - Create
- **`immybot-pp-cli smtp-configs create-by-tenant-id`** - Create by tenant id
- **`immybot-pp-cli smtp-configs create-send-test-email`** - Create send test email
- **`immybot-pp-cli smtp-configs delete-by-tenant-id`** - Delete by tenant id
- **`immybot-pp-cli smtp-configs get-by-tenant-id`** - Get by tenant id
- **`immybot-pp-cli smtp-configs list`** - List

### software

Manage software

- **`immybot-pp-cli software create-global`** - Create global
- **`immybot-pp-cli software create-global-analyze`** - Create global analyze
- **`immybot-pp-cli software create-global-by-identifier-versions`** - Create global by identifier versions
- **`immybot-pp-cli software create-global-fast-create`** - Create global fast create
- **`immybot-pp-cli software create-global-upload`** - Create global upload
- **`immybot-pp-cli software create-local`** - Create local
- **`immybot-pp-cli software create-local-analyze`** - Create local analyze
- **`immybot-pp-cli software create-local-by-identifier-authorization`** - Create local by identifier authorization
- **`immybot-pp-cli software create-local-by-identifier-migrate-local-to-global`** - Create local by identifier migrate local to global
- **`immybot-pp-cli software create-local-by-identifier-versions`** - Create local by identifier versions
- **`immybot-pp-cli software create-local-fast-create`** - Create local fast create
- **`immybot-pp-cli software create-local-upload`** - Create local upload
- **`immybot-pp-cli software delete-global-by-identifier`** - Delete global by identifier
- **`immybot-pp-cli software delete-global-by-identifier-versions-by-semantic-version`** - Delete global by identifier versions by semantic version
- **`immybot-pp-cli software delete-local-by-identifier`** - Delete local by identifier
- **`immybot-pp-cli software delete-local-by-identifier-versions-by-semantic-version`** - Delete local by identifier versions by semantic version
- **`immybot-pp-cli software get-global`** - Get global
- **`immybot-pp-cli software get-global-by-identifier`** - Get global by identifier
- **`immybot-pp-cli software get-global-by-identifier-latest`** - Get global by identifier latest
- **`immybot-pp-cli software get-global-by-identifier-versions`** - Get global by identifier versions
- **`immybot-pp-cli software get-global-by-identifier-versions-by-semantic-version`** - Get global by identifier versions by semantic version
- **`immybot-pp-cli software get-global-by-identifier-versions-by-semantic-version-request-download`** - Get global by identifier versions by semantic version request download
- **`immybot-pp-cli software get-local`** - Get local
- **`immybot-pp-cli software get-local-by-identifier`** - Get local by identifier
- **`immybot-pp-cli software get-local-by-identifier-authorization`** - Get local by identifier authorization
- **`immybot-pp-cli software get-local-by-identifier-latest`** - Get local by identifier latest
- **`immybot-pp-cli software get-local-by-identifier-migrate-local-to-global-what-if`** - Get local by identifier migrate local to global what if
- **`immybot-pp-cli software get-local-by-identifier-versions`** - Get local by identifier versions
- **`immybot-pp-cli software get-local-by-identifier-versions-by-semantic-version`** - Get local by identifier versions by semantic version
- **`immybot-pp-cli software get-local-by-identifier-versions-by-semantic-version-request-download`** - Get local by identifier versions by semantic version request download
- **`immybot-pp-cli software update-global-by-identifier`** - Update global by identifier
- **`immybot-pp-cli software update-global-by-identifier-versions-by-semantic-version`** - Update global by identifier versions by semantic version
- **`immybot-pp-cli software update-local-by-identifier`** - Update local by identifier
- **`immybot-pp-cli software update-local-by-identifier-versions-by-semantic-version`** - Update local by identifier versions by semantic version

### syncs

Manage syncs

- **`immybot-pp-cli syncs create-azure-user`** - Create azure user
- **`immybot-pp-cli syncs create-expire-pending-sessions`** - Create expire pending sessions
- **`immybot-pp-cli syncs create-trigger-user-affinity`** - Create trigger user affinity

### system

Manage system

- **`immybot-pp-cli system create-disable-immy-support-access`** - Create disable immy support access
- **`immybot-pp-cli system create-enable-immy-support-access`** - Create enable immy support access
- **`immybot-pp-cli system create-is-immy-support-access-granted`** - Create is immy support access granted
- **`immybot-pp-cli system create-pull-update`** - Create pull update
- **`immybot-pp-cli system create-request-form-support`** - Create request form support
- **`immybot-pp-cli system create-request-session-support`** - Create request session support
- **`immybot-pp-cli system create-restart-backend`** - Create restart backend
- **`immybot-pp-cli system create-update-release-channel`** - Create update release channel
- **`immybot-pp-cli system get-immy-support-access-grant-details`** - Get immy support access grant details
- **`immybot-pp-cli system get-releases`** - Get releases
- **`immybot-pp-cli system get-timezones`** - Get timezones

### tags

Manage tags

- **`immybot-pp-cli tags create`** - Create
- **`immybot-pp-cli tags create-by-id`** - Create by id
- **`immybot-pp-cli tags delete-by-id`** - Delete by id
- **`immybot-pp-cli tags get-by-id`** - Get by id
- **`immybot-pp-cli tags list`** - List

### target-assignments

Manage target assignments

- **`immybot-pp-cli target-assignments create`** - Create
- **`immybot-pp-cli target-assignments create-change-request-by-change-request-id-v2`** - Create change request by change request id v2
- **`immybot-pp-cli target-assignments create-change-request-v2`** - Create change request v2
- **`immybot-pp-cli target-assignments create-duplicate`** - Create duplicate
- **`immybot-pp-cli target-assignments create-duplicates`** - Create duplicates
- **`immybot-pp-cli target-assignments create-global-by-id-notes`** - Create global by id notes
- **`immybot-pp-cli target-assignments create-global-by-id-override`** - Create global by id override
- **`immybot-pp-cli target-assignments create-global-create`** - Create global create
- **`immybot-pp-cli target-assignments create-migrate-deployments-to-provider-links`** - Create migrate deployments to provider links
- **`immybot-pp-cli target-assignments create-migrate-to-superseding-assignment`** - Create migrate to superseding assignment
- **`immybot-pp-cli target-assignments create-migrate-to-superseding-assignment-what-if`** - Create migrate to superseding assignment what if
- **`immybot-pp-cli target-assignments create-optional-approvals-by-id`** - Create optional approvals by id
- **`immybot-pp-cli target-assignments create-persons-target-preview`** - Create persons target preview
- **`immybot-pp-cli target-assignments create-recommended-approvals-update`** - Create recommended approvals update
- **`immybot-pp-cli target-assignments create-target-preview`** - Create target preview
- **`immybot-pp-cli target-assignments create-tenant-target-preview`** - Create tenant target preview
- **`immybot-pp-cli target-assignments create-update-maintenance-item-order`** - Create update maintenance item order
- **`immybot-pp-cli target-assignments create-visibility`** - Create visibility
- **`immybot-pp-cli target-assignments delete-by-id`** - Delete by id
- **`immybot-pp-cli target-assignments delete-global-by-id`** - Delete global by id
- **`immybot-pp-cli target-assignments get-by-id`** - Get by id
- **`immybot-pp-cli target-assignments get-change-request-by-change-request-id`** - Get change request by change request id
- **`immybot-pp-cli target-assignments get-change-request-by-change-request-id-diff`** - Get change request by change request id diff
- **`immybot-pp-cli target-assignments get-change-requests`** - Get change requests
- **`immybot-pp-cli target-assignments get-global`** - Get global
- **`immybot-pp-cli target-assignments get-global-by-id`** - Get global by id
- **`immybot-pp-cli target-assignments get-global-by-id-type`** - Get global by id type
- **`immybot-pp-cli target-assignments get-maintenance-item-orders`** - Get maintenance item orders
- **`immybot-pp-cli target-assignments get-optional-approvals-computer-by-computer-id`** - Get optional approvals computer by computer id
- **`immybot-pp-cli target-assignments get-recommended-approvals`** - Get recommended approvals
- **`immybot-pp-cli target-assignments list`** - List
- **`immybot-pp-cli target-assignments update-batch-update`** - Update batch update
- **`immybot-pp-cli target-assignments update-by-id`** - Update by id
- **`immybot-pp-cli target-assignments update-global-by-id`** - Update global by id

### tenants

Manage tenants

- **`immybot-pp-cli tenants create`** - Create
- **`immybot-pp-cli tenants create-add-tags`** - Create add tags
- **`immybot-pp-cli tenants create-bulk-create`** - Create bulk create
- **`immybot-pp-cli tenants create-bulk-delete`** - Create bulk delete
- **`immybot-pp-cli tenants create-bulk-merge`** - Create bulk merge
- **`immybot-pp-cli tenants create-remove-parent`** - Create remove parent
- **`immybot-pp-cli tenants create-remove-tags`** - Create remove tags
- **`immybot-pp-cli tenants create-resolve-assignments-for-maintenance-item`** - Create resolve assignments for maintenance item
- **`immybot-pp-cli tenants create-set-parent`** - Create set parent
- **`immybot-pp-cli tenants create-update-azure-link`** - Create update azure link
- **`immybot-pp-cli tenants get-by-id`** - Get by id
- **`immybot-pp-cli tenants get-computer-counts`** - Get computer counts
- **`immybot-pp-cli tenants get-excluded-from-cross-deployments`** - Get excluded from cross deployments
- **`immybot-pp-cli tenants get-software-from-inventory-by-id`** - Get software from inventory by id
- **`immybot-pp-cli tenants get-software-from-inventory-dx`** - Get software from inventory dx
- **`immybot-pp-cli tenants get-software-from-inventory-export`** - Streams the contents of the detected computer software table as a CSV file to the client
- **`immybot-pp-cli tenants list`** - List
- **`immybot-pp-cli tenants update-activate-by-id`** - Update activate by id
- **`immybot-pp-cli tenants update-by-id`** - Update by id
- **`immybot-pp-cli tenants update-deactivate-by-id`** - Update deactivate by id

### user-role-assignments

Manage user role assignments

- **`immybot-pp-cli user-role-assignments create-category-resource-create`** - Create category resource create
- **`immybot-pp-cli user-role-assignments create-msp-create`** - Create msp create
- **`immybot-pp-cli user-role-assignments create-owner-create`** - Create owner create
- **`immybot-pp-cli user-role-assignments create-specific-resource-create`** - Create specific resource create
- **`immybot-pp-cli user-role-assignments create-specific-tenant-create`** - Create specific tenant create
- **`immybot-pp-cli user-role-assignments create-tag-resource-create`** - Create tag resource create
- **`immybot-pp-cli user-role-assignments create-tenant-tag-create`** - Create tenant tag create
- **`immybot-pp-cli user-role-assignments create-user-tenant-create`** - Create user tenant create
- **`immybot-pp-cli user-role-assignments delete-delete`** - Delete delete
- **`immybot-pp-cli user-role-assignments get-users-by-user-id`** - Get users by user id
- **`immybot-pp-cli user-role-assignments get-users-by-user-id-count`** - Get users by user id count
- **`immybot-pp-cli user-role-assignments list`** - List

### user_session

Manage user session

- **`immybot-pp-cli user-session get-login`** - Get login
- **`immybot-pp-cli user-session get-logout`** - Get logout
- **`immybot-pp-cli user-session get-me`** - Get me
- **`immybot-pp-cli user-session get-refresh`** - Get refresh

### users

Manage users

- **`immybot-pp-cli users create-bulk-create`** - Create bulk create
- **`immybot-pp-cli users create-by-id`** - Create by id
- **`immybot-pp-cli users create-invalidate-cache`** - Create invalidate cache
- **`immybot-pp-cli users create-stop-impersonating`** - Create stop impersonating
- **`immybot-pp-cli users create-submit-feedback`** - Create submit feedback
- **`immybot-pp-cli users create-update-expiration`** - Create update expiration
- **`immybot-pp-cli users delete-bulk-delete`** - Delete bulk delete
- **`immybot-pp-cli users delete-by-id`** - Delete by id
- **`immybot-pp-cli users get-by-id`** - Get by id
- **`immybot-pp-cli users get-claims`** - Get claims
- **`immybot-pp-cli users list`** - List

### webhooks

Manage webhooks

- **`immybot-pp-cli webhooks create-by-id`** - Create by id
- **`immybot-pp-cli webhooks get-by-id`** - Get by id


### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`immybot-pp-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`immybot-pp-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`immybot-pp-cli learnings list`** - Inspect taught rows
- **`immybot-pp-cli learnings forget <query>`** - Undo a teach
- **`immybot-pp-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`immybot-pp-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`immybot-pp-cli teach-pattern`** - Install a query/resource template up front
- **`immybot-pp-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `IMMYBOT_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `immybot-pp-cli` opens the database, older binaries refuse it with a version error — upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
immybot-pp-cli access list

# JSON for scripting and agents
immybot-pp-cli access list --json
# Filter to specific fields
immybot-pp-cli access list --json --select addons,backendRegAppId,canManageCrossTenantDeployments

# Dry run — show the request without sending
immybot-pp-cli access list --dry-run

# Agent mode — JSON + compact + no prompts in one flag
immybot-pp-cli access list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **Explicit confirmation** - `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Runtime Endpoint

This CLI resolves endpoint placeholders at runtime, so one installed binary can target different tenants or API versions without regeneration.

Endpoint environment variables:
- `IMMYBOT_SUBDOMAIN` resolves `{subdomain}`

Base URL: `https://{subdomain}.immy.bot`

## Health Check

```bash
immybot-pp-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `immybot-pp-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/immy-bot-pp-cli/config.toml`; `--home`, `IMMYBOT_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `IMMYBOT_SUBDOMAIN` | endpoint | Yes |  |
| `IMMYBOT_TENANT_ID` | auth_flow_input | Yes | Microsoft Entra directory (tenant) ID that issues the token. |
| `IMMYBOT_CLIENT_ID` | auth_flow_input | Yes | Application (client) ID of the Entra app registration. |
| `IMMYBOT_CLIENT_SECRET` | auth_flow_input | Yes | Set during initial auth setup. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `immybot-pp-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `immybot-pp-cli doctor` to check credentials
- Verify the environment variable is set: `echo $IMMYBOT_TENANT_ID`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **Every request returns 401 Unauthorized** — Confirm the Enterprise Application object ID (not the Application ID) is set as a Person's AD External ID in ImmyBot and that the person is an admin user.
- **tenant required for token URL** — Set IMMYBOT_TENANT_ID to the Entra directory (tenant) ID that issues the token.
- **Requests go to your-instance.immy.bot** — Set IMMYBOT_SUBDOMAIN to your instance name without the .immy.bot suffix, or pass --base-url.
- **Token mints but the API still rejects it** — The scope must match your instance App ID URI; set IMMYBOT_OAUTH_SCOPE to https://<subdomain>.immy.bot/.default.
- **Cross-tenant commands return nothing** — Run sync --resources tenants,computers,software first; those commands read the local mirror, not the live API.
- **HTTP 429 Too Many Requests** — Lower sync concurrency by syncing one resource at a time, or re-run with --max-pages to bound the walk.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**ImmyBotApiWrapper**](https://github.com/serialscriptr/ImmyBotApiWrapper) — PowerShell (4 stars)
- [**immybot-vscode**](https://github.com/immense/immybot-vscode) — TypeScript (3 stars)
- [**claude-immybot-skill**](https://github.com/dillon-LACT/claude-immybot-skill) — PowerShell (2 stars)
- [**immybot-mcp**](https://github.com/wyre-technology/immybot-mcp) — TypeScript
- [**SPSImmyBot**](https://github.com/suhsdit/SPSImmyBot) — PowerShell
- [**n8n-nodes-immybot**](https://github.com/n8layer/n8n-nodes-immybot) — TypeScript
- [**Bezalu.ImmyBot.Client**](https://github.com/BezaluLLC/Bezalu.ImmyBot.Client) — C#
- [**Bezalu.ImmyBot.MCP**](https://github.com/BezaluLLC/Bezalu.ImmyBot.MCP) — Dockerfile

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
