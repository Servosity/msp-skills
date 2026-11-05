# Rewst CLI

**Every Rewst entity as a typed command, plus fleet health, ROI, drift and coverage rollups**

Rewst's only surface is a GraphQL gateway with no first-party CLI and no local state. This CLI mirrors organizations, workflows, executions, variables, packs, triggers, forms, and the rest as typed commands with --json/--select/--agent output, the `api` command that lists every GraphQL interface and method, and offline SQLite sync. On top of that it adds multi-org rollups  -  fleet execution health, failure triage, automation ROI, dormant-workflow detection, cross-org config drift, and pack coverage  -  that an MSP operator otherwise has to assemble by clicking through the web app one client at a time.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `rewst-cli` and `rewst-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

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

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=rewst). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/rewst --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/rewst --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `rewst-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the rewst skill from https://github.com/Servosity/msp-skills/tree/main/skills/rewst. The skill defines how its required CLI (`rewst-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=rewst).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `REWST_API_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rewst/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rewst/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "rewst": {
      "command": "rewst-mcp",
      "env": {
        "REWST_API_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`rewst-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `REWST_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `REWST_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export REWST_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
rewst-mcp --transport http
```

Set `REWST_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

## Authentication

Rewst authenticates with a per-organization API client token sent as `Authorization: Bearer <token>`. Mint a token in the Rewst platform under Configuration > API Clients, then export it as `REWST_API_TOKEN` (or run `rewst-cli auth set-token`). The token is org-scoped, so most commands take an `--org` id for the tenant you are operating on.

## Quick Start

```bash
# Confirm the CLI is wired up and see whether REWST_API_TOKEN is set before making real calls.
rewst-cli doctor --dry-run

# List the org ids this token can see (the --org values every rollup takes).
rewst-cli orgs --agent

# Get the execution health verdict for the last day.
rewst-cli health --org YOUR_ORG_ID --since 24h --agent

# Pull the recent failed runs to triage.
rewst-cli failures --org YOUR_ORG_ID --since 24h --limit 20 --agent

# Roll up the time automation saved this month.
rewst-cli roi --org YOUR_ORG_ID --since 30d --agent

```

## Unique Features

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

## Usage

Run `rewst-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `REWST_CONFIG_DIR`, `REWST_DATA_DIR`, `REWST_STATE_DIR`, or `REWST_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `REWST_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export REWST_HOME=/srv/rewst
rewst-cli doctor
```

On Windows (PowerShell):

```powershell
$env:REWST_HOME = "C:\ProgramData\rewst"
rewst-cli doctor
```

Under `REWST_HOME=/srv/rewst`, the four dirs resolve to `/srv/rewst/config`, `/srv/rewst/data`, `/srv/rewst/state`, and `/srv/rewst/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:REWST_HOME = "C:\srv\rewst"
rewst-cli doctor
```

Under `$env:REWST_HOME = "C:\srv\rewst"`, the four dirs resolve to `C:\srv\rewst\config`, `C:\srv\rewst\data`, `C:\srv\rewst\state`, and `C:\srv\rewst\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

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

Precedence matters in fleets: an ambient per-kind variable such as `REWST_DATA_DIR` overrides an explicit `--home` for that kind. Use `REWST_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `REWST_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `rewst-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are never cached on disk. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `rewst-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `rewst-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `REWST_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `REWST_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### action-options

Manage action-options

- **`rewst-cli action-options create`** - Create a actionoption
- **`rewst-cli action-options get`** - Get a single actionoption

### actions

Manage actions

- **`rewst-cli actions create`** - Create a action
- **`rewst-cli actions get`** - Get a single action

### active-conversation-requests

Manage active-conversation-requests

- **`rewst-cli active-conversation-requests`** - Get a single activeconversationrequest

### affected-workflow-infos

Manage affected-workflow-infos

- **`rewst-cli affected-workflow-infos`** - Get a single affectedworkflowinfo

### api-client-lists

Manage api-client-lists

- **`rewst-cli api-client-lists`** - Get a single apiclientlist

### api-clients

Manage api-clients

- **`rewst-cli api-clients <id>`** - Get a single apiclient

### app-platform-reserved-domains

Manage app-platform-reserved-domains

- **`rewst-cli app-platform-reserved-domains create`** - Create a appplatformreserveddomain
- **`rewst-cli app-platform-reserved-domains get`** - Get a single appplatformreserveddomain
- **`rewst-cli app-platform-reserved-domains update`** - Update a appplatformreserveddomain

### commonly-used-actions

Manage commonly-used-actions

- **`rewst-cli commonly-used-actions`** - Get a single commonlyusedaction

### component-instances

Manage component-instances

- **`rewst-cli component-instances create`** - Create a componentinstance
- **`rewst-cli component-instances get`** - Get a single componentinstance
- **`rewst-cli component-instances update`** - Update a componentinstance

### component-trees

Manage component-trees

- **`rewst-cli component-trees`** - Get a single componenttree

### components

Manage components

- **`rewst-cli components`** - Get a single component

### conversation-message-votes

Manage conversation-message-votes

- **`rewst-cli conversation-message-votes create`** - Create a conversationmessagevote
- **`rewst-cli conversation-message-votes get`** - Get a single conversationmessagevote
- **`rewst-cli conversation-message-votes update`** - Update a conversationmessagevote

### conversation-messages

Manage conversation-messages

- **`rewst-cli conversation-messages`** - Create a conversationmessage

### conversations

Manage conversations

- **`rewst-cli conversations create`** - Create a conversation
- **`rewst-cli conversations get`** - Get a single conversation
- **`rewst-cli conversations update`** - Update a conversation

### crate-override-options

Manage crate-override-options

- **`rewst-cli crate-override-options create`** - Create a crateoverrideoption
- **`rewst-cli crate-override-options update`** - Update a crateoverrideoption

### crate-overrides

Manage crate-overrides

- **`rewst-cli crate-overrides create`** - Create a crateoverride
- **`rewst-cli crate-overrides update`** - Update a crateoverride

### crate-unpacking-argument-sets

Manage crate-unpacking-argument-sets

- **`rewst-cli crate-unpacking-argument-sets`** - Get a single crateunpackingargumentset

### crate-use-cases

Manage crate-use-cases

- **`rewst-cli crate-use-cases`** - Get a single crateusecase

### crates

Manage crates

- **`rewst-cli crates create`** - Create a crate
- **`rewst-cli crates get`** - Get a single crate
- **`rewst-cli crates update`** - Update a crate

### database-notification-errors

Manage database-notification-errors

- **`rewst-cli database-notification-errors`** - Get a single databasenotificationerror

### delete-org-interpreter-responses

Manage delete-org-interpreter-responses

- **`rewst-cli delete-org-interpreter-responses <id>`** - Delete a deleteorginterpreterresponse

### dropdown-options

Manage dropdown-options

- **`rewst-cli dropdown-options`** - Get a single dropdownoption

### encoded-page-nodeses

Manage encoded-page-nodeses

- **`rewst-cli encoded-page-nodeses <id>`** - Get a single encodedpagenodes

### feature-preview-settings

Manage feature-preview-settings

- **`rewst-cli feature-preview-settings create`** - Create a featurepreviewsetting
- **`rewst-cli feature-preview-settings get`** - Get a single featurepreviewsetting
- **`rewst-cli feature-preview-settings update`** - Update a featurepreviewsetting

### foreign-object-references

Manage foreign-object-references

- **`rewst-cli foreign-object-references`** - Get a single foreignobjectreference

### form-audit-entries

Manage form-audit-entries

- **`rewst-cli form-audit-entries`** - Get a single formauditentry

### form-permission-states

Manage form-permission-states

- **`rewst-cli form-permission-states`** - Get a single formpermissionstate

### forms

Manage forms

- **`rewst-cli forms create`** - Create a form
- **`rewst-cli forms get`** - Get a single form
- **`rewst-cli forms update`** - Update a form

### integration-workflow-outputs

Manage integration-workflow-outputs

- **`rewst-cli integration-workflow-outputs`** - Create a integrationworkflowoutput

### integrations

Manage integrations

- **`rewst-cli integrations`** - Get a single integration

### interpreter-versions

Manage interpreter-versions

- **`rewst-cli interpreter-versions`** - Get a single interpreterversion

### jinja-render-sessions

Manage jinja-render-sessions

- **`rewst-cli jinja-render-sessions <id>`** - Get a single jinjarendersession

### jinja2-documentations

Manage jinja2-documentations

- **`rewst-cli jinja2-documentations`** - Get a single jinja2documentation

### job-requested-responses

Manage job-requested-responses

- **`rewst-cli job-requested-responses`** - Create a jobrequestedresponse

### logins

Manage logins

- **`rewst-cli logins`** - Get a single login

### message-vote-statses

Manage message-vote-statses

- **`rewst-cli message-vote-statses`** - Get a single messagevotestats

### microsoft-c-s-p-customers

Manage microsoft-c-s-p-customers

- **`rewst-cli microsoft-c-s-p-customers`** - Get a single microsoftcspcustomer

### monaco-completion-items

Manage monaco-completion-items

- **`rewst-cli monaco-completion-items`** - Get a single monacocompletionitem

### onboarding-questionnaire-responses

Manage onboarding-questionnaire-responses

- **`rewst-cli onboarding-questionnaire-responses create`** - Create a onboardingquestionnaireresponse
- **`rewst-cli onboarding-questionnaire-responses get`** - Get a single onboardingquestionnaireresponse

### org-breadcrumbs

Manage org-breadcrumbs

- **`rewst-cli org-breadcrumbs`** - Get a single orgbreadcrumb

### org-form-field-instances

Manage org-form-field-instances

- **`rewst-cli org-form-field-instances`** - Get a single orgformfieldinstance

### org-interpreter-settings

Manage org-interpreter-settings

- **`rewst-cli org-interpreter-settings`** - Get a single orginterpretersetting

### org-search-results

Manage org-search-results

- **`rewst-cli org-search-results`** - Get a single orgsearchresult

### org-support-accesses

Manage org-support-accesses

- **`rewst-cli org-support-accesses`** - Create a orgsupportaccess

### org-trigger-instances

Manage org-trigger-instances

- **`rewst-cli org-trigger-instances get`** - Get a single orgtriggerinstance
- **`rewst-cli org-trigger-instances update`** - Update a orgtriggerinstance

### org-variables

Manage org-variables

- **`rewst-cli org-variables create`** - Create a orgvariable
- **`rewst-cli org-variables get`** - Get a single orgvariable
- **`rewst-cli org-variables update`** - Update a orgvariable

### organization-audit-entries

Manage organization-audit-entries

- **`rewst-cli organization-audit-entries`** - Get a single organizationauditentry

### organization-imports

Manage organization-imports

- **`rewst-cli organization-imports`** - Get a single organizationimport

### organization-onboarding-crate-requirements

Manage organization-onboarding-crate-requirements

- **`rewst-cli organization-onboarding-crate-requirements`** - Get a single organizationonboardingcraterequirement

### organization-onboarding-pack-requirements

Manage organization-onboarding-pack-requirements

- **`rewst-cli organization-onboarding-pack-requirements`** - Get a single organizationonboardingpackrequirement

### organization-onboarding-requirements

Manage organization-onboarding-requirements

- **`rewst-cli organization-onboarding-requirements create`** - Create a organizationonboardingrequirement
- **`rewst-cli organization-onboarding-requirements get`** - Get a single organizationonboardingrequirement

### organizations

Manage organizations

- **`rewst-cli organizations create`** - Create a organization
- **`rewst-cli organizations get`** - Get a single organization
- **`rewst-cli organizations update`** - Update a organization

### pack-action-options

Manage pack-action-options

- **`rewst-cli pack-action-options`** - Get a single packactionoption

### pack-bundles

Manage pack-bundles

- **`rewst-cli pack-bundles`** - Get a single packbundle

### pack-configs

Manage pack-configs

- **`rewst-cli pack-configs create`** - Create a packconfig
- **`rewst-cli pack-configs get`** - Get a single packconfig
- **`rewst-cli pack-configs update`** - Update a packconfig

### pack-delete-responses

Manage pack-delete-responses

- **`rewst-cli pack-delete-responses`** - Delete a packdeleteresponse

### pack-resource-types-containers

Manage pack-resource-types-containers

- **`rewst-cli pack-resource-types-containers`** - Get a single packresourcetypescontainer

### packs

Manage packs

- **`rewst-cli packs create`** - Create a pack
- **`rewst-cli packs get`** - Get a single pack
- **`rewst-cli packs update`** - Update a pack

### packs-and-bundles-by-installed-states

Manage packs-and-bundles-by-installed-states

- **`rewst-cli packs-and-bundles-by-installed-states`** - Get a single packsandbundlesbyinstalledstate

### page-nodes

Manage page-nodes

- **`rewst-cli page-nodes`** - Get a single pagenode

### pages

Manage pages

- **`rewst-cli pages create`** - Create a page
- **`rewst-cli pages get`** - Get a single page
- **`rewst-cli pages update`** - Update a page

### pending-tasks-aggregates

Manage pending-tasks-aggregates

- **`rewst-cli pending-tasks-aggregates`** - Get a single pendingtasksaggregate

### permission-audit-log-lists

Manage permission-audit-log-lists

- **`rewst-cli permission-audit-log-lists`** - Get a single permissionauditloglist

### permissions

Manage permissions

- **`rewst-cli permissions create`** - Create a permission
- **`rewst-cli permissions get`** - Get a single permission
- **`rewst-cli permissions update`** - Update a permission

### psa-filter-optionses

Manage psa-filter-optionses

- **`rewst-cli psa-filter-optionses`** - Get a single psafilteroptions

### psa-organizations

Manage psa-organizations

- **`rewst-cli psa-organizations`** - Get a single psaorganization

### public-crates

Manage public-crates

- **`rewst-cli public-crates`** - Get a single publiccrate

### reserved-organization-names

Manage reserved-organization-names

- **`rewst-cli reserved-organization-names`** - Get a single reservedorganizationname

### robo-rewsty-config-values

Manage robo-rewsty-config-values

- **`rewst-cli robo-rewsty-config-values`** - Get a single roborewstyconfigvalue

### role-organization-counts

Manage role-organization-counts

- **`rewst-cli role-organization-counts`** - Get a single roleorganizationcount

### role-organization-rows

Manage role-organization-rows

- **`rewst-cli role-organization-rows`** - Get a single roleorganizationrow

### role-user-counts

Manage role-user-counts

- **`rewst-cli role-user-counts`** - Get a single roleusercount

### roles

Manage roles

- **`rewst-cli roles create`** - Create a role
- **`rewst-cli roles get`** - Get a single role
- **`rewst-cli roles update`** - Update a role

### sensor-types

Manage sensor-types

- **`rewst-cli sensor-types`** - Get a single sensortype

### site-domain-valids

Manage site-domain-valids

- **`rewst-cli site-domain-valids`** - Get a single sitedomainvalid

### sites

Manage sites

- **`rewst-cli sites create`** - Create a site
- **`rewst-cli sites get`** - Get a single site
- **`rewst-cli sites update`** - Update a site

### spice-d-b-check-results

Manage spice-d-b-check-results

- **`rewst-cli spice-d-b-check-results`** - Get a single spicedbcheckresult

### tags

Manage tags

- **`rewst-cli tags create`** - Create a tag
- **`rewst-cli tags get`** - Get a single tag
- **`rewst-cli tags update`** - Update a tag

### task-count-by-dates

Manage task-count-by-dates

- **`rewst-cli task-count-by-dates`** - Get a single taskcountbydate

### task-count-by-hours

Manage task-count-by-hours

- **`rewst-cli task-count-by-hours`** - Get a single taskcountbyhour

### task-logs

Manage task-logs

- **`rewst-cli task-logs create`** - Create a tasklog
- **`rewst-cli task-logs get`** - Get a single tasklog

### templates

Manage templates

- **`rewst-cli templates create`** - Create a template
- **`rewst-cli templates get`** - Get a single template
- **`rewst-cli templates update`** - Update a template

### time-saved-by-dates

Manage time-saved-by-dates

- **`rewst-cli time-saved-by-dates`** - Get a single timesavedbydate

### time-saved-by-hours

Manage time-saved-by-hours

- **`rewst-cli time-saved-by-hours`** - Get a single timesavedbyhour

### time-saved-group-by-orgs

Manage time-saved-group-by-orgs

- **`rewst-cli time-saved-group-by-orgs`** - Get a single timesavedgroupbyorg

### time-saved-group-by-workflows

Manage time-saved-group-by-workflows

- **`rewst-cli time-saved-group-by-workflows`** - Get a single timesavedgroupbyworkflow

### trigger-types

Manage trigger-types

- **`rewst-cli trigger-types`** - Get a single triggertype

### triggers

Manage triggers

- **`rewst-cli triggers create`** - Create a trigger
- **`rewst-cli triggers get`** - Get a single trigger
- **`rewst-cli triggers update`** - Update a trigger

### user-delegated-accesses

Manage user-delegated-accesses

- **`rewst-cli user-delegated-accesses`** - Get a single userdelegatedaccess

### user-favorite-actions

Manage user-favorite-actions

- **`rewst-cli user-favorite-actions create`** - Create a userfavoriteaction
- **`rewst-cli user-favorite-actions update`** - Update a userfavoriteaction

### user-invites

Manage user-invites

- **`rewst-cli user-invites create`** - Create a userinvite
- **`rewst-cli user-invites get`** - Get a single userinvite

### user-preferenceses

Manage user-preferenceses

- **`rewst-cli user-preferenceses`** - Update a userpreferences

### user-robo-rewsty-preferenceses

Manage user-robo-rewsty-preferenceses

- **`rewst-cli user-robo-rewsty-preferenceses get`** - Get a single userroborewstypreferences
- **`rewst-cli user-robo-rewsty-preferenceses update`** - Update a userroborewstypreferences

### users

Manage users

- **`rewst-cli users create`** - Create a user
- **`rewst-cli users get`** - Get a single user
- **`rewst-cli users update`** - Update a user

### workflow-execution-statses

Manage workflow-execution-statses

- **`rewst-cli workflow-execution-statses`** - Get a single workflowexecutionstats

### workflow-executions

Manage workflow-executions

- **`rewst-cli workflow-executions`** - Get a single workflowexecution

### workflow-notes

Manage workflow-notes

- **`rewst-cli workflow-notes`** - Get a single workflownote

### workflow-patches

Manage workflow-patches

- **`rewst-cli workflow-patches`** - Get a single workflowpatch

### workflow-stats-by-orgs

Manage workflow-stats-by-orgs

- **`rewst-cli workflow-stats-by-orgs`** - Get a single workflowstatsbyorg

### workflow-tasks

Manage workflow-tasks

- **`rewst-cli workflow-tasks`** - Get a single workflowtask

### workflows

Manage workflows

- **`rewst-cli workflows create`** - Create a workflow
- **`rewst-cli workflows get`** - Get a single workflow
- **`rewst-cli workflows update`** - Update a workflow


### coverage


- **`rewst-cli coverage`** - Across a parent org's managed sub-orgs, shows which integration packs are installed where and flags orgs missing a pack  -  bounded by a scan cap.


### dormant


- **`rewst-cli dormant`** - Workflows that have not executed in N days  -  dead or orphaned automation that is still installed but no longer running.


### drift


- **`rewst-cli drift`** - Compares org variables and installed packs between two organizations and reports what one has that the other is missing.


### failures


- **`rewst-cli failures`** - Recent failed workflow executions for an org, newest first, with workflow id, status, task counts, and age  -  the triage queue the API has no single endpoint for.


### health


- **`rewst-cli health`** - One-call workflow execution health for an org: succeeded / failed / running / pending counts plus time saved, with a clear unhealthy verdict when failures are present.


### roi


- **`rewst-cli roi`** - Aggregates Rewst's humanSecondsSaved metric across an org's workflows into total hours/days saved and the top time-saving automations.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`rewst-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`rewst-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`rewst-cli learnings list`** - Inspect taught rows
- **`rewst-cli learnings forget <query>`** - Undo a teach
- **`rewst-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`rewst-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`rewst-cli teach-pattern`** - Install a query/resource template up front
- **`rewst-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `REWST_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `rewst-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
rewst-cli action-options get

# JSON for scripting and agents
rewst-cli action-options get --json
# Filter to specific fields
rewst-cli action-options get --json --select actions,id,optionLabel

# Dry run  -  show the request without sending
rewst-cli action-options get --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
rewst-cli action-options get --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

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

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `8` async job submitted but not finished (rerun the printed recovery command; do not resubmit), `10` config error.

## Health Check

```bash
rewst-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `rewst-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/rewst-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\rewst-cli\config.toml` on Windows; `--home`, `REWST_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `REWST_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `REWST_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `REWST_API_TOKEN` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `rewst-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Credential safety

- The API token (`Authorization: Bearer <REWST_API_TOKEN>`) and every API request go only to the scheme, host and port of the configured base URL (`https://api.rewst.io`, or `REWST_BASE_URL` for the EU and AU regions; every operation is `POST /graphql` there). A redirect to any other host is refused, so the token never reaches it.
- Any output, log, file, local-store row or MCP result that would repeat `REWST_API_TOKEN`, the stored token, or a credential a response carries (an API client's `auth0ClientSecret` and `auth0ClientId`, a third-party login token, an integration secret inside a pack config) is replaced by `[credkit: withheld from <where>: it carried a credential this program holds]`, in plain text and in the encodings the shared credential kit decodes. Responses are never cached on disk.
- A `--stdin` body is held whole, so a write's answer that repeats part of the body prints that notice too.
- Not held yet: the value of an org variable whose category is `secret` prints as Rewst returns it. Treat `org-variables` output as sensitive.
- Only `credentials.toml` (mode 0600) holds a credential; `auth set-token` reads the token from stdin, never from an argument.
- A GraphQL answer that carries an `errors` member is reported as a failure, never as empty data.
- `--dry-run` sends nothing. Over MCP, `rewst_execute` (every Rewst operation that changes something), `import` and `jobs_prune` return a preview and send only with `confirm: true`, and a tool argument can never name a file to read or write.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `rewst-cli doctor` to check credentials
- Verify the environment variable is set: `echo $REWST_API_TOKEN`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **A typed entity command (organizations, workflows, org-variables, ...) fails with an HTTP 400 or a GraphQL error from /graphql**  -  Known limitation of this build: the typed entity commands are generated as REST-shaped calls the Rewst GraphQL gateway refuses, and a refused or errored GraphQL answer is reported as a failure, never as empty data. Use the GraphQL-native commands orgs, health, failures, roi, dormant, drift and coverage, which send real GraphQL queries.
- **Commands return a GraphQL authentication or 'not authorized' error**  -  Set a valid org-scoped token: export REWST_API_TOKEN=<token from Configuration > API Clients>, or run rewst-cli auth set-token.
- **A query returns no data even with a valid token**  -  Pass the correct tenant with --org <orgId>; most Rewst queries are organization-scoped and return nothing for the wrong org.
- **health / roi / coverage report empty results**  -  Confirm the org has workflows and executions in the window; widen --since or --days, and verify the token's org can see managed sub-orgs for coverage.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**RewstApp/agent-smith**](https://github.com/RewstApp/agent-smith)  -  Go
- [**Tre-Eiler/Rewst**](https://github.com/Tre-Eiler/Rewst)  -  Other

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
