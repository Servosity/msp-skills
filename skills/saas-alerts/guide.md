# SaaS Alerts CLI

**Covers the SaaS Alerts Reports, Manage, Respond, and Webhooks APIs with one partner key, mints the Respond access token for you, and adds cross-tenant triage and a Respond approval queue with deadlines.**

One partner API key reaches all four SaaS Alerts APIs, and the CLI exchanges it for the Respond access token on first use, so Respond calls work from scripts and agents without a browser session. Sync customers, events, and Respond data into a local store, then run triage, cross-tenant, investigate, and coverage across every customer, and work the Respond queue with rule-triggers queue and the preview-first rule-triggers decide.

Learn more at [SaaS Alerts](https://saasalerts.com).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `saas-alerts-cli` and `saas-alerts-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/saas-alerts/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/saas-alerts/install.ps1 | iex
   ```
3. Verify: `saas-alerts-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=saas-alerts). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`. On Windows, take the Windows build; if PowerShell reports the downloaded file as blocked, run `Unblock-File <binary>.exe`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/saas-alerts --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/saas-alerts --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `saas-alerts-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the saas-alerts skill from https://github.com/Servosity/msp-skills/tree/main/skills/saas-alerts. The skill defines how its required CLI (`saas-alerts-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=saas-alerts).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `SAAS_ALERTS_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/saas-alerts/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/saas-alerts/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`%APPDATA%\Claude\claude_desktop_config.json` on Windows, `~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "saas-alerts": {
      "command": "saas-alerts-mcp",
      "env": {
        "SAAS_ALERTS_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Create the key in SaaS Alerts under Settings > API (Show key under Manage API) and set it as SAAS_ALERTS_API_KEY (macOS/Linux: `export SAAS_ALERTS_API_KEY=<key>`; PowerShell: `$env:SAAS_ALERTS_API_KEY = "<key>"`). The same key works for the Reports, Manage, and Webhooks APIs. Respond calls need an extra access token: the CLI exchanges your key for one via POST /respondApi/api/v1/auth on first use, sends it as the accesstoken header, and re-mints it once if Respond rejects it. If every call returns 401 Access denied, ask SaaS Alerts support to enable the REST API for your account. SaaS Alerts publishes only the US API host; if your account is on another SaaS Alerts instance, set SAAS_ALERTS_BASE_URL to that host.

## Quick Start

```bash
# Preview the health check without calling the API; drop --dry-run to test the key
saas-alerts-cli doctor --dry-run

# Pull a week of events plus the Respond data the local commands join
saas-alerts-cli sync --resources customers,events,rule-triggers,suppressions,devices,organizations,rules --since 7d

# What fired across every customer, critical first
saas-alerts-cli triage --since 24h --agent

# Respond triggers waiting on you, soonest deadline first
saas-alerts-cli rule-triggers queue --agent

# Drill into one account from the triage list
saas-alerts-cli investigate jane.doe@example.com --since 7d --agent

```

## Unique Features

### Cross-tenant triage
- **`triage`**  -  See what fired across every customer in a window, critical first, grouped by customer and user, with ticket state and pull lag.

  _Reach for this first in a morning or shift-start triage instead of paging events per customer._

  ```bash
  saas-alerts-cli triage --since 24h --min-severity medium --agent
  ```
- **`cross-tenant`**  -  Find IPs, ASNs, or countries hitting two or more customers in a window, minus each customer's own whitelist for that dimension.

  _Use when one attacker may be spraying several customers._

  ```bash
  saas-alerts-cli cross-tenant --by ip --since 24h --min-customers 2 --agent
  ```
- **`investigate`**  -  Everything known about one account: events, country hops, IPs and ASNs, user record, Respond triggers, suppressions, and mapped devices.

  _Use for one suspicious user instead of pivoting across five endpoints._

  ```bash
  saas-alerts-cli investigate jane.doe@example.com --since 7d --hop-window 2h --agent
  ```

### Respond, safely
- **`rule-triggers queue`**  -  List pending and failed Respond triggers by how soon they expire or auto-ignore, with the actions an approval would fire.

  _Use before any approve or reject so the decision that will be lost first is handled first._

  ```bash
  saas-alerts-cli rule-triggers queue --expiring-within 2h --agent
  ```
- **`rule-triggers decide`**  -  Approve, reject, ignore, or manually remediate a filtered set of Respond triggers, previewed first and verified after.

  _Use to apply one decision to many triggers; nothing is sent without --confirm._

  ```bash
  saas-alerts-cli rule-triggers decide --decision reject --rule RULE_ID_12345 --remarks "false positive" --agent
  ```
- **`contain`**  -  Block sign-in, expire sessions, and reset the password for one Microsoft 365 account in order, previewed first, then follow the jobs.

  _Use for a compromised Microsoft 365 account with no Respond trigger; nothing runs without --confirm, and IDs come from synced rule triggers or users unless you pass --org-id/--account-id._

  ```bash
  saas-alerts-cli contain jane.doe@example.com --actions block,expire,reset --agent
  ```

### Blind spots
- **`subscriptions health`**  -  Check every webhook subscription's state, unsent queue, and domain, flag the update trap, and plan re-enabling disabled ones (sent only with --reenable --confirm).

  _Use when tickets stop arriving, or on a heartbeat schedule._

  ```bash
  saas-alerts-cli subscriptions health --agent
  ```
- **`coverage`**  -  Flag customers that went silent, lost their Respond connection, or have no Respond rules.

  _Use weekly to find customers where monitoring or auto-remediation is quietly off._

  ```bash
  saas-alerts-cli coverage --silent 72h --agent
  ```

## Recipes

### Critical events, narrowed for an agent

```bash
saas-alerts-cli events list --alert-status critical --size 100 --agent --select time,customer.name,user.name,jointDesc,location.country
```

Events are large nested documents; --select keeps only the fields triage needs.

### Shift-start triage

```bash
saas-alerts-cli triage --since 12h --min-severity medium --agent
```

Severity-ordered digest across every synced customer with ticket state.

### One source, many customers

```bash
saas-alerts-cli cross-tenant --by ip --since 24h --min-customers 2 --agent
```

Finds an IP seen in two or more tenants' events, excluding whitelisted ranges.

### Work the Respond queue

```bash
saas-alerts-cli rule-triggers queue --expiring-within 2h --agent
```

Pending and failed triggers ranked by the deadline that will be lost first.

### Webhook heartbeat

```bash
saas-alerts-cli subscriptions health --agent
```

Lists disabled subscriptions, unsent queue depth, and unverified domains; add --reenable to preview the fix.

## Usage

Run `saas-alerts-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `SAAS_ALERTS_CONFIG_DIR`, `SAAS_ALERTS_DATA_DIR`, `SAAS_ALERTS_STATE_DIR`, or `SAAS_ALERTS_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `SAAS_ALERTS_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export SAAS_ALERTS_HOME=/srv/saas-alerts
saas-alerts-cli doctor
```

On Windows (PowerShell):

```powershell
$env:SAAS_ALERTS_HOME = "C:\ProgramData\saas-alerts"
saas-alerts-cli doctor
```

Under `SAAS_ALERTS_HOME=/srv/saas-alerts`, the four dirs resolve to `/srv/saas-alerts/config`, `/srv/saas-alerts/data`, `/srv/saas-alerts/state`, and `/srv/saas-alerts/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "saas-alerts": {
      "command": "saas-alerts-mcp",
      "env": {
        "SAAS_ALERTS_HOME": "/srv/saas-alerts"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `SAAS_ALERTS_DATA_DIR` overrides an explicit `--home` for that kind. Use `SAAS_ALERTS_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `SAAS_ALERTS_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `saas-alerts-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### alert-types

Live catalog of SaaS Alerts alert/event types (Webhooks API)

- **`saas-alerts-cli alert-types`** - Get List of Alert Types

### application-connections

Respond application connections (Respond API)

- **`saas-alerts-cli application-connections connect`** - Connect or Reconnect an application connection. Pass a random nanoId if connecting
- **`saas-alerts-cli application-connections delete`** - Delete an application connection

### applications

Respond applications (Respond API)

- **`saas-alerts-cli applications get`** - Retrieve application by id
- **`saas-alerts-cli applications list`** - Retrieve applications that will help you retrieve the client ids to do oauth connections

### customers

Customer organizations: list, get, create, update, delete, whitelists

- **`saas-alerts-cli customers create`** - Create customer for the authenticated partner
- **`saas-alerts-cli customers delete`** - Delete customer for the authenticated partner
- **`saas-alerts-cli customers get`** - Retrieve customer by customer id
- **`saas-alerts-cli customers list`** - Retrieve a list of customers for the authenticated partner
- **`saas-alerts-cli customers set-account-whitelists`** - Set whitelists for the specified customer accounts
- **`saas-alerts-cli customers set-whitelists`** - Customer - Set whitelists for the customer
- **`saas-alerts-cli customers update`** - Update customer fields for the authenticated partner

### devices

Unify device-to-account mapping (Manage API)

- **`saas-alerts-cli devices ignored`** - Retrieves a list of ignored devices based on the specified query parameters.
- **`saas-alerts-cli devices involved`** - Retrieves a list of devices associated with the specified accounts based on the given query parameters.
- **`saas-alerts-cli devices list`** - Retrieves a list of devices based on the specified query parameters.
- **`saas-alerts-cli devices map`** - Maps an account to all or specific devices with account suggestions.
- **`saas-alerts-cli devices mapped`** - Retrieves a list of mapped devices based on the specified query parameters.
- **`saas-alerts-cli devices mapped-to`** - Retrieves a list of devices that are mapped to the specified accounts based on the given query parameters.
- **`saas-alerts-cli devices suggested`** - Retrieves a list of devices that are suggested for an account based on the specified criteria.
- **`saas-alerts-cli devices unmap`** - Removes an account from specific devices or all devices where it is currently mapped.
- **`saas-alerts-cli devices unmapped`** - Retrieves a list of unmapped devices based on the specified query parameters.

### events

Security events: list, query, count, scroll, and file-share activity

- **`saas-alerts-cli events count`** - Retrieve a count of events using query parameters
- **`saas-alerts-cli events count-query`** - Retrieve a count of events for a specific query
- **`saas-alerts-cli events fileshare`** - Retrieve file share events with optional filtering by organization id and grouping by file name
- **`saas-alerts-cli events list`** - Retrieve events by Customer and additional query values
- **`saas-alerts-cli events query`** - Retrieve events for specific query
- **`saas-alerts-cli events scroll`** - Fetch the next page of a scroll cursor started by 'events query --scroll 5m' (pass --scroll-id and a keepalive)

### files

Report file storage (Reports API)

- **`saas-alerts-cli files delete`** - Remove uploaded file from cloud storage
- **`saas-alerts-cli files upload`** - Upload a file to cloud storage
- **`saas-alerts-cli files url`** - Get signed url of the cloud storage object

### ioc-triggers

Respond IOC triggers (Respond API)

- **`saas-alerts-cli ioc-triggers <id>`** - Retrieve an IOC trigger by id

### iocs

Respond IOC rules (Respond API)

- **`saas-alerts-cli iocs archive`** - Archive an IOC rule by id
- **`saas-alerts-cli iocs draft`** - Upsert a draft IOC rule by id
- **`saas-alerts-cli iocs duplicate`** - Duplicate an IOC rule by id
- **`saas-alerts-cli iocs get`** - Retrieve IOC rule by id
- **`saas-alerts-cli iocs list`** - Gets all IOC rules (no pagination)
- **`saas-alerts-cli iocs unarchive`** - Unarchive an IOC rule by id
- **`saas-alerts-cli iocs update`** - Upsert an IOC rule by id

### organizations

Respond organization connections (Respond API)

- **`saas-alerts-cli organizations connections`** - Retrieve list of applications for current organization
- **`saas-alerts-cli organizations disconnected`** - Get only disconnected organizations (includes not connected, broken and upgradable organizations)
- **`saas-alerts-cli organizations list`** - Retrieve list of organizations connections for current partner

### partner-users

Partner (MSP) users (Reports API)

- **`saas-alerts-cli partner-users`** - Partner users

### properties

Respond rule-condition property catalogs (Respond API)

- **`saas-alerts-cli properties account`** - Get Account Property configurations to configure Account based rules
- **`saas-alerts-cli properties event`** - Get allowed event properties filter to filter each event.

### psa

PSA customer mapping (Manage API)

- **`saas-alerts-cli psa map`** - Adds a PSA customer mapping.
- **`saas-alerts-cli psa map-suggested`** - This is a beta feature, and is known to time out when there is a large list of companies in the PSA. If you experience a timeout issue and this is a critical feature for you, contact support@kaseya.com

### reports

Partner, customer, MFA, mailbox-rule, billing, and scheduled reports

- **`saas-alerts-cli reports alert-recommended-actions`** - Alert recommended actions
- **`saas-alerts-cli reports billing-dates`** - Billing dates
- **`saas-alerts-cli reports billing-details`** - partner - partner details & list customers, users, billable users against a customer
- **`saas-alerts-cli reports customer`** - Retrieve customer by customer id
- **`saas-alerts-cli reports customer-item`** - Retrieve customer by customer id
- **`saas-alerts-cli reports customers`** - Retrieve a list of customers for the authenticated partner
- **`saas-alerts-cli reports devices-organizations`** - Get devices organizations
- **`saas-alerts-cli reports mailbox-rules`** - account-mailbox-rules for report app.
- **`saas-alerts-cli reports mfa-google`** - Get google workspace mfasettings
- **`saas-alerts-cli reports mfa-microsoft`** - Get microsoft mfasettings
- **`saas-alerts-cli reports msp-user`** - Retrieve the MSP user: id, name, email, role, partner id, status
- **`saas-alerts-cli reports partner-profile`** - Retrieve the partner profile: name, address, phone
- **`saas-alerts-cli reports scheduled-create`** - Add a scheduled report to the database
- **`saas-alerts-cli reports scheduled-delete`** - Delete scheduled report by scheduledReportId
- **`saas-alerts-cli reports scheduled-get`** - Retrieve a scheduled report by scheduledReportId
- **`saas-alerts-cli reports scheduled-list`** - Retrieve all the scheduled reports of the partner
- **`saas-alerts-cli reports send-email`** - Send email via mailgun
- **`saas-alerts-cli reports unify-ignored`** - unify-ignored-devices  for report app.
- **`saas-alerts-cli reports unify-mapped`** - unify-mapped-devices-by-account for report app.
- **`saas-alerts-cli reports unify-unmapped`** - unify-unmapped-devices-by-account for report app.
- **`saas-alerts-cli reports update-branding`** - Update the partner branding (logo, brand color)

### respond

Respond on/off state for the partner (Respond API)

- **`saas-alerts-cli respond disable`** - Disable respond state for current partner.
- **`saas-alerts-cli respond enable`** - Enable respond state for current partner.
- **`saas-alerts-cli respond status`** - Retrieve respond state for current partner.

### response-actions

Respond response actions and their jobs (Respond API)

- **`saas-alerts-cli response-actions execute`** - Execute a response action for one or more accounts and create a job
- **`saas-alerts-cli response-actions job`** - retrieve response action job by id
- **`saas-alerts-cli response-actions jobs`** - retrieve response actions jobs

### rule-templates

Respond rule templates (Respond API)

- **`saas-alerts-cli rule-templates get`** - Retrieve rule template by id
- **`saas-alerts-cli rule-templates list`** - Gets all rules (no pagination)
- **`saas-alerts-cli rule-templates use`** - Create a draft rule using rule template

### rule-triggers

Respond rule triggers: the approval queue (Respond API)

- **`saas-alerts-cli rule-triggers approve`** - Approve a rule trigger
- **`saas-alerts-cli rule-triggers get`** - Retrieve a rule trigger by id
- **`saas-alerts-cli rule-triggers ignore`** - Ignore a rule trigger
- **`saas-alerts-cli rule-triggers list`** - Get all rule triggers or filter by parameters
- **`saas-alerts-cli rule-triggers manually-remediate`** - Manually Remediate a rule trigger
- **`saas-alerts-cli rule-triggers reject`** - Reject a rule trigger

### rules

Respond rules (Respond API)

- **`saas-alerts-cli rules convert-to-ioc`** - Convert a rule to an IOC rule by rule id
- **`saas-alerts-cli rules delete`** - Delete a rule
- **`saas-alerts-cli rules disconnected-orgs`** - Get disconnected organizations selected in the rule
- **`saas-alerts-cli rules draft`** - Upsert a draft rule by id
- **`saas-alerts-cli rules duplicate`** - Duplicate a rule by id
- **`saas-alerts-cli rules get`** - Retrieve rule by id
- **`saas-alerts-cli rules list`** - Gets all rules (no pagination)
- **`saas-alerts-cli rules update`** - Upsert a rule by id

### subscriptions

Processed-event webhook subscriptions (Webhooks API)

- **`saas-alerts-cli subscriptions create`** - Subscription - create item
- **`saas-alerts-cli subscriptions customers`** - Get List of Customers
- **`saas-alerts-cli subscriptions delete`** - Subscription - Delete item
- **`saas-alerts-cli subscriptions disabled`** - Retrieve list of disabled subscriptions for current partner.
- **`saas-alerts-cli subscriptions list`** - Retrieve list of active subscriptions for current partner.
- **`saas-alerts-cli subscriptions test-event`** - Subscription - Test Event
- **`saas-alerts-cli subscriptions update`** - Subscription - Update item

### suppressions

Alert suppression rules (Manage API)

- **`saas-alerts-cli suppressions create`** - Creates a new suppression rule.
- **`saas-alerts-cli suppressions delete`** - Deletes a suppression rule.
- **`saas-alerts-cli suppressions event-types`** - Retrieves available event types and their associated products for suppression conditions.
- **`saas-alerts-cli suppressions fields`** - Retrieves the suppression field configuration (condition form schema).
- **`saas-alerts-cli suppressions get`** - Retrieves a specific suppression rule by ID.
- **`saas-alerts-cli suppressions list`** - Retrieves all suppression rules for the authenticated partner.
- **`saas-alerts-cli suppressions update`** - Updates an existing suppression rule.

### users

Users of one customer organization (Reports API)

- **`saas-alerts-cli users`** - Retrieve a list of users by Customer

### webhook-domains

Verified webhook receiver domains (Webhooks API)

- **`saas-alerts-cli webhook-domains delete`** - Delete a verified webhook domain
- **`saas-alerts-cli webhook-domains list`** - List verified webhook domains
- **`saas-alerts-cli webhook-domains verify`** - Verify a webhook receiver domain and save it on success

### webhook-queue

Unsent webhook event packs (Webhooks API)

- **`saas-alerts-cli webhook-queue channel`** - Retrieve list of unsent packs of events for current channel.
- **`saas-alerts-cli webhook-queue delete`** - Delete channel from queue
- **`saas-alerts-cli webhook-queue events`** - Retrieve list of events for current channel.
- **`saas-alerts-cli webhook-queue list`** - Retrieve list of unsent packs of events for current partner.


### contain


- **`saas-alerts-cli contain`** - Block sign-in, expire sessions, and reset the password for one Microsoft 365 account in order, previewed first, then follow the jobs.


### coverage


- **`saas-alerts-cli coverage`** - Flag customers that went silent, lost their Respond connection, or have no Respond rules.


### cross-tenant


- **`saas-alerts-cli cross-tenant`** - Find IPs, ASNs, or countries hitting two or more customers in a window, minus each customer's own whitelist for that dimension.


### investigate


- **`saas-alerts-cli investigate`** - Everything known about one account: events, country hops, IPs and ASNs, user record, Respond triggers, suppressions, and mapped devices.


### rule-triggers


- **`saas-alerts-cli rule-triggers queue`** - List pending and failed Respond triggers by how soon they expire or auto-ignore, with the actions an approval would fire.

- **`saas-alerts-cli rule-triggers decide`** - Approve, reject, ignore, or manually remediate a filtered set of Respond triggers, previewed first and verified after.


### subscriptions


- **`saas-alerts-cli subscriptions health`** - Check every webhook subscription's state, unsent queue, and domain, flag the update trap, and plan re-enabling disabled ones (sent only with --reenable --confirm).


### triage


- **`saas-alerts-cli triage`** - See what fired across every customer in a window, critical first, grouped by customer and user, with ticket state and pull lag.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`saas-alerts-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`saas-alerts-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`saas-alerts-cli learnings list`** - Inspect taught rows
- **`saas-alerts-cli learnings forget <query>`** - Undo a teach
- **`saas-alerts-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`saas-alerts-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`saas-alerts-cli teach-pattern`** - Install a query/resource template up front
- **`saas-alerts-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `SAAS_ALERTS_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `saas-alerts-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
saas-alerts-cli alert-types

# JSON for scripting and agents
saas-alerts-cli alert-types --json
# Filter to specific fields by name
saas-alerts-cli customers list --json --select id,name

# Dry run  -  show the request without sending
saas-alerts-cli alert-types --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
saas-alerts-cli alert-types --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **Writes** - generated create, update, and delete commands send immediately (add `--dry-run` to preview); `rule-triggers decide`, `contain`, and `subscriptions health --reenable` only preview until you pass `--confirm`
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

## Freshness

This CLI owns bounded freshness for registered store-backed read command paths. In `--data-source auto` mode, covered commands check the local SQLite store before serving results; stale or missing resources trigger a bounded refresh, and refresh failures fall back to the existing local data with a warning. `--data-source local` never refreshes, and `--data-source live` reads the API without mutating the local store.

Set `SAAS_ALERTS_NO_AUTO_REFRESH=1` to disable the pre-read freshness hook while preserving the selected data source.

Covered command paths:
- `saas-alerts-cli applications`
- `saas-alerts-cli applications get`
- `saas-alerts-cli applications list`
- `saas-alerts-cli coverage`
- `saas-alerts-cli cross-tenant`
- `saas-alerts-cli customers`
- `saas-alerts-cli customers get`
- `saas-alerts-cli customers list`
- `saas-alerts-cli devices`
- `saas-alerts-cli devices list`
- `saas-alerts-cli events`
- `saas-alerts-cli events list`
- `saas-alerts-cli investigate`
- `saas-alerts-cli iocs`
- `saas-alerts-cli iocs get`
- `saas-alerts-cli iocs list`
- `saas-alerts-cli organizations`
- `saas-alerts-cli organizations list`
- `saas-alerts-cli partner-users`
- `saas-alerts-cli properties`
- `saas-alerts-cli response-actions`
- `saas-alerts-cli rule-templates`
- `saas-alerts-cli rule-templates get`
- `saas-alerts-cli rule-templates list`
- `saas-alerts-cli rule-triggers`
- `saas-alerts-cli rule-triggers get`
- `saas-alerts-cli rule-triggers list`
- `saas-alerts-cli rules`
- `saas-alerts-cli rules get`
- `saas-alerts-cli rules list`
- `saas-alerts-cli subscriptions`
- `saas-alerts-cli subscriptions list`
- `saas-alerts-cli suppressions`
- `saas-alerts-cli suppressions get`
- `saas-alerts-cli suppressions list`
- `saas-alerts-cli triage`
- `saas-alerts-cli webhook-domains`
- `saas-alerts-cli webhook-domains list`
- `saas-alerts-cli webhook-queue`
- `saas-alerts-cli webhook-queue list`

JSON outputs that use the generated provenance envelope include freshness metadata at `meta.freshness`. This metadata describes the freshness decision for the covered command path; it does not claim full historical backfill or API-specific enrichment.

## Health Check

```bash
saas-alerts-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `saas-alerts-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/saas-alerts-cli/config.toml` (`%USERPROFILE%\.config\saas-alerts-cli\config.toml` on Windows); `--home`, `SAAS_ALERTS_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `SAAS_ALERTS_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `saas-alerts-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `saas-alerts-cli doctor` to check credentials
- Check where the key comes from without printing it: `saas-alerts-cli auth status`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **Respond calls fail with 400 Only secure origins are allowed**  -  That is a missing access token, not CORS. This CLI mints one automatically; run saas-alerts-cli respond status --agent to exercise the mint, and check SAAS_ALERTS_API_KEY and that Respond is enabled for the partner.
- **events query returns 422 'body' is required**  -  Wrap the Elasticsearch search in an envelope: {"body": {"query": ...}, "scroll": "5m"} and pass it with --stdin.
- **Event lists stop at 10 rows**  -  Pass --size (the search backend defaults to 10); the API silently ignores unknown names like limit or offset.
- **Every call returns 401 Access denied**  -  Check the key from Settings > API, and ask SaaS Alerts support to enable the REST API for the account.
- **Account is on the EU instance**  -  Set SAAS_ALERTS_BASE_URL to your instance's host.
- **triage, investigate, or coverage return empty results**  -  They read the local store; run saas-alerts-cli sync --resources customers,events,rule-triggers,suppressions,devices,organizations,rules --since 7d first.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**bjones-saasalerts/n8n**](https://github.com/bjones-saasalerts/n8n)  -  JavaScript (1 stars)
- [**WYRE-AI/saas-alerts-mcp**](https://github.com/WYRE-AI/saas-alerts-mcp)  -  TypeScript
- [**am3cramirez/saas-alerts-mcp**](https://github.com/am3cramirez/saas-alerts-mcp)  -  TypeScript
- [**InfoSecGeoff/SaaSquatch**](https://github.com/InfoSecGeoff/SaaSquatch)  -  PowerShell
- [**WYRE-AI/node-saas-alerts**](https://github.com/WYRE-AI/node-saas-alerts)  -  TypeScript
- [**JM-1683/SaaSMap-server**](https://github.com/JM-1683/SaaSMap-server)  -  JavaScript

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
