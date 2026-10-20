---
name: saas-alerts
description: "Covers the SaaS Alerts Reports, Manage, Respond, and Webhooks APIs with one partner key, mints the Respond access token for you, and adds cross-tenant triage and a Respond approval queue with deadlines. Trigger phrases: `what fired overnight in SaaS Alerts`, `triage SaaS Alerts critical alerts`, `approve pending Respond triggers`, `investigate this user in SaaS Alerts`, `re-enable SaaS Alerts webhooks`, `use saas-alerts`, `run saas-alerts`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "SaaS Alerts"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - saas-alerts-cli
    install:
      - kind: script
        bins: [saas-alerts-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/saas-alerts/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/saas-alerts/install.ps1
---

# SaaS Alerts  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `saas-alerts-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

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

The installer downloads the `saas-alerts-cli` and `saas-alerts-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

One partner API key reaches all four SaaS Alerts APIs, and the CLI exchanges it for the Respond access token on first use, so Respond calls work from scripts and agents without a browser session. Sync customers, events, and Respond data into a local store, then run triage, cross-tenant, investigate, and coverage across every customer, and work the Respond queue with rule-triggers queue and the preview-first rule-triggers decide.

## When to Use This CLI

Use this CLI when an agent needs SaaS Alerts data or actions across many customer tenants: overnight triage, investigating one account, correlating a source across customers, working the Respond approval queue, or checking webhook feed health. It is the right tool for scripted or scheduled MSP security operations against M365 and Google Workspace activity that SaaS Alerts already monitors.

## Anti-triggers

Do not use this CLI for:
- Do not use it to receive webhooks in real time; point a webhook endpoint at SaaS Alerts and use subscriptions health only to check that feed.
- Do not use it to manage PSA tickets; use the PSA's own connector (ConnectWise, Autotask, HaloPSA).
- Do not use it for general Microsoft 365 or Entra administration beyond Respond's response actions; use Microsoft Graph or CIPP.
- Do not use it for RMM device management; Unify device mapping here only links devices to accounts.

## Unique Capabilities

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

## Command Reference

**alert-types**  -  Live catalog of SaaS Alerts alert/event types (Webhooks API)

- `saas-alerts-cli alert-types`  -  Get List of Alert Types

**application-connections**  -  Respond application connections (Respond API)

- `saas-alerts-cli application-connections connect`  -  Connect or Reconnect an application connection. Pass a random nanoId if connecting
- `saas-alerts-cli application-connections delete`  -  Delete an application connection

**applications**  -  Respond applications (Respond API)

- `saas-alerts-cli applications get`  -  Retrieve application by id
- `saas-alerts-cli applications list`  -  Retrieve applications that will help you retrieve the client ids to do oauth connections

**customers**  -  Customer organizations: list, get, create, update, delete, whitelists

- `saas-alerts-cli customers create`  -  Create customer for the authenticated partner
- `saas-alerts-cli customers delete`  -  Delete customer for the authenticated partner
- `saas-alerts-cli customers get`  -  Retrieve customer by customer id
- `saas-alerts-cli customers list`  -  Retrieve a list of customers for the authenticated partner
- `saas-alerts-cli customers set-account-whitelists`  -  Set whitelists for the specified customer accounts
- `saas-alerts-cli customers set-whitelists`  -  Customer - Set whitelists for the customer
- `saas-alerts-cli customers update`  -  Update customer fields for the authenticated partner

**devices**  -  Unify device-to-account mapping (Manage API)

- `saas-alerts-cli devices ignored`  -  Retrieves a list of ignored devices based on the specified query parameters.
- `saas-alerts-cli devices involved`  -  Retrieves a list of devices associated with the specified accounts based on the given query parameters.
- `saas-alerts-cli devices list`  -  Retrieves a list of devices based on the specified query parameters.
- `saas-alerts-cli devices map`  -  Maps an account to all or specific devices with account suggestions.
- `saas-alerts-cli devices mapped`  -  Retrieves a list of mapped devices based on the specified query parameters.
- `saas-alerts-cli devices mapped-to`  -  Retrieves a list of devices that are mapped to the specified accounts based on the given query parameters.
- `saas-alerts-cli devices suggested`  -  Retrieves a list of devices that are suggested for an account based on the specified criteria.
- `saas-alerts-cli devices unmap`  -  Removes an account from specific devices or all devices where it is currently mapped.
- `saas-alerts-cli devices unmapped`  -  Retrieves a list of unmapped devices based on the specified query parameters.

**events**  -  Security events: list, query, count, scroll, and file-share activity

- `saas-alerts-cli events count`  -  Retrieve a count of events using query parameters
- `saas-alerts-cli events count-query`  -  Retrieve a count of events for a specific query
- `saas-alerts-cli events fileshare`  -  Retrieve file share events with optional filtering by organization id and grouping by file name
- `saas-alerts-cli events list`  -  Retrieve events by Customer and additional query values
- `saas-alerts-cli events query`  -  Retrieve events for specific query
- `saas-alerts-cli events scroll`  -  Fetch the next page of a scroll cursor started by 'events query --scroll 5m' (pass --scroll-id and a keepalive)

**files**  -  Report file storage (Reports API)

- `saas-alerts-cli files delete`  -  Remove uploaded file from cloud storage
- `saas-alerts-cli files upload`  -  Upload a file to cloud storage
- `saas-alerts-cli files url`  -  Get signed url of the cloud storage object

**ioc-triggers**  -  Respond IOC triggers (Respond API)

- `saas-alerts-cli ioc-triggers <id>`  -  Retrieve an IOC trigger by id

**iocs**  -  Respond IOC rules (Respond API)

- `saas-alerts-cli iocs archive`  -  Archive an IOC rule by id
- `saas-alerts-cli iocs draft`  -  Upsert a draft IOC rule by id
- `saas-alerts-cli iocs duplicate`  -  Duplicate an IOC rule by id
- `saas-alerts-cli iocs get`  -  Retrieve IOC rule by id
- `saas-alerts-cli iocs list`  -  Gets all IOC rules (no pagination)
- `saas-alerts-cli iocs unarchive`  -  Unarchive an IOC rule by id
- `saas-alerts-cli iocs update`  -  Upsert an IOC rule by id

**organizations**  -  Respond organization connections (Respond API)

- `saas-alerts-cli organizations connections`  -  Retrieve list of applications for current organization
- `saas-alerts-cli organizations disconnected`  -  Get only disconnected organizations (includes not connected, broken and upgradable organizations)
- `saas-alerts-cli organizations list`  -  Retrieve list of organizations connections for current partner

**partner-users**  -  Partner (MSP) users (Reports API)

- `saas-alerts-cli partner-users`  -  Partner users

**properties**  -  Respond rule-condition property catalogs (Respond API)

- `saas-alerts-cli properties account`  -  Get Account Property configurations to configure Account based rules
- `saas-alerts-cli properties event`  -  Get allowed event properties filter to filter each event.

**psa**  -  PSA customer mapping (Manage API)

- `saas-alerts-cli psa map`  -  Adds a PSA customer mapping.
- `saas-alerts-cli psa map-suggested`  -  This is a beta feature, and is known to time out when there is a large list of companies in the PSA.

**reports**  -  Partner, customer, MFA, mailbox-rule, billing, and scheduled reports

- `saas-alerts-cli reports alert-recommended-actions`  -  Alert recommended actions
- `saas-alerts-cli reports billing-dates`  -  Billing dates
- `saas-alerts-cli reports billing-details`  -  partner - partner details & list customers, users, billable users against a customer
- `saas-alerts-cli reports customer`  -  Retrieve customer by customer id
- `saas-alerts-cli reports customer-item`  -  Retrieve customer by customer id
- `saas-alerts-cli reports customers`  -  Retrieve a list of customers for the authenticated partner
- `saas-alerts-cli reports devices-organizations`  -  Get devices organizations
- `saas-alerts-cli reports mailbox-rules`  -  account-mailbox-rules for report app.
- `saas-alerts-cli reports mfa-google`  -  Get google workspace mfasettings
- `saas-alerts-cli reports mfa-microsoft`  -  Get microsoft mfasettings
- `saas-alerts-cli reports msp-user`  -  Retrieve the MSP user: id, name, email, role, partner id, status
- `saas-alerts-cli reports partner-profile`  -  Retrieve the partner profile: name, address, phone
- `saas-alerts-cli reports scheduled-create`  -  Add a scheduled report to the database
- `saas-alerts-cli reports scheduled-delete`  -  Delete scheduled report by scheduledReportId
- `saas-alerts-cli reports scheduled-get`  -  Retrieve a scheduled report by scheduledReportId
- `saas-alerts-cli reports scheduled-list`  -  Retrieve all the scheduled reports of the partner
- `saas-alerts-cli reports send-email`  -  Send email via mailgun
- `saas-alerts-cli reports unify-ignored`  -  unify-ignored-devices for report app.
- `saas-alerts-cli reports unify-mapped`  -  unify-mapped-devices-by-account for report app.
- `saas-alerts-cli reports unify-unmapped`  -  unify-unmapped-devices-by-account for report app.
- `saas-alerts-cli reports update-branding`  -  Update the partner branding (logo, brand color)

**respond**  -  Respond on/off state for the partner (Respond API)

- `saas-alerts-cli respond disable`  -  Disable respond state for current partner.
- `saas-alerts-cli respond enable`  -  Enable respond state for current partner.
- `saas-alerts-cli respond status`  -  Retrieve respond state for current partner.

**response-actions**  -  Respond response actions and their jobs (Respond API)

- `saas-alerts-cli response-actions execute`  -  Execute a response action for one or more accounts and create a job
- `saas-alerts-cli response-actions job`  -  retrieve response action job by id
- `saas-alerts-cli response-actions jobs`  -  retrieve response actions jobs

**rule-templates**  -  Respond rule templates (Respond API)

- `saas-alerts-cli rule-templates get`  -  Retrieve rule template by id
- `saas-alerts-cli rule-templates list`  -  Gets all rules (no pagination)
- `saas-alerts-cli rule-templates use`  -  Create a draft rule using rule template

**rule-triggers**  -  Respond rule triggers: the approval queue (Respond API)

- `saas-alerts-cli rule-triggers approve`  -  Approve a rule trigger
- `saas-alerts-cli rule-triggers get`  -  Retrieve a rule trigger by id
- `saas-alerts-cli rule-triggers ignore`  -  Ignore a rule trigger
- `saas-alerts-cli rule-triggers list`  -  Get all rule triggers or filter by parameters
- `saas-alerts-cli rule-triggers manually-remediate`  -  Manually Remediate a rule trigger
- `saas-alerts-cli rule-triggers reject`  -  Reject a rule trigger

**rules**  -  Respond rules (Respond API)

- `saas-alerts-cli rules convert-to-ioc`  -  Convert a rule to an IOC rule by rule id
- `saas-alerts-cli rules delete`  -  Delete a rule
- `saas-alerts-cli rules disconnected-orgs`  -  Get disconnected organizations selected in the rule
- `saas-alerts-cli rules draft`  -  Upsert a draft rule by id
- `saas-alerts-cli rules duplicate`  -  Duplicate a rule by id
- `saas-alerts-cli rules get`  -  Retrieve rule by id
- `saas-alerts-cli rules list`  -  Gets all rules (no pagination)
- `saas-alerts-cli rules update`  -  Upsert a rule by id

**subscriptions**  -  Processed-event webhook subscriptions (Webhooks API)

- `saas-alerts-cli subscriptions create`  -  Subscription - create item
- `saas-alerts-cli subscriptions customers`  -  Get List of Customers
- `saas-alerts-cli subscriptions delete`  -  Subscription - Delete item
- `saas-alerts-cli subscriptions disabled`  -  Retrieve list of disabled subscriptions for current partner.
- `saas-alerts-cli subscriptions list`  -  Retrieve list of active subscriptions for current partner.
- `saas-alerts-cli subscriptions test-event`  -  Subscription - Test Event
- `saas-alerts-cli subscriptions update`  -  Subscription - Update item

**suppressions**  -  Alert suppression rules (Manage API)

- `saas-alerts-cli suppressions create`  -  Creates a new suppression rule.
- `saas-alerts-cli suppressions delete`  -  Deletes a suppression rule.
- `saas-alerts-cli suppressions event-types`  -  Retrieves available event types and their associated products for suppression conditions.
- `saas-alerts-cli suppressions fields`  -  Retrieves the suppression field configuration (condition form schema).
- `saas-alerts-cli suppressions get`  -  Retrieves a specific suppression rule by ID.
- `saas-alerts-cli suppressions list`  -  Retrieves all suppression rules for the authenticated partner.
- `saas-alerts-cli suppressions update`  -  Updates an existing suppression rule.

**users**  -  Users of one customer organization (Reports API)

- `saas-alerts-cli users`  -  Retrieve a list of users by Customer

**webhook-domains**  -  Verified webhook receiver domains (Webhooks API)

- `saas-alerts-cli webhook-domains delete`  -  Delete a verified webhook domain
- `saas-alerts-cli webhook-domains list`  -  List verified webhook domains
- `saas-alerts-cli webhook-domains verify`  -  Verify a webhook receiver domain and save it on success

**webhook-queue**  -  Unsent webhook event packs (Webhooks API)

- `saas-alerts-cli webhook-queue channel`  -  Retrieve list of unsent packs of events for current channel.
- `saas-alerts-cli webhook-queue delete`  -  Delete channel from queue
- `saas-alerts-cli webhook-queue events`  -  Retrieve list of events for current channel.
- `saas-alerts-cli webhook-queue list`  -  Retrieve list of unsent packs of events for current partner.


**contain**


- `saas-alerts-cli contain`  -  Block sign-in, expire sessions, and reset the password for one Microsoft 365 account in order, previewed first, then follow the jobs.


**coverage**


- `saas-alerts-cli coverage`  -  Flag customers that went silent, lost their Respond connection, or have no Respond rules.


**cross-tenant**


- `saas-alerts-cli cross-tenant`  -  Find IPs, ASNs, or countries hitting two or more customers in a window, minus each customer's own whitelist for that dimension.


**investigate**


- `saas-alerts-cli investigate`  -  Everything known about one account: events, country hops, IPs and ASNs, user record, Respond triggers, suppressions, and mapped devices.


**rule-triggers**


- `saas-alerts-cli rule-triggers queue`  -  List pending and failed Respond triggers by how soon they expire or auto-ignore, with the actions an approval would fire.

- `saas-alerts-cli rule-triggers decide`  -  Approve, reject, ignore, or manually remediate a filtered set of Respond triggers, previewed first and verified after.


**subscriptions**


- `saas-alerts-cli subscriptions health`  -  Check every webhook subscription's state, unsent queue, and domain, flag the update trap, and plan re-enabling disabled ones (sent only with --reenable --confirm).


**triage**


- `saas-alerts-cli triage`  -  See what fired across every customer in a window, critical first, grouped by customer and user, with ticket state and pull lag.




## Freshness Contract

This printed CLI owns bounded freshness only for registered store-backed read command paths. In `--data-source auto` mode, those paths check `sync_state` and may run a bounded refresh before reading local data. `--data-source local` never refreshes. `--data-source live` reads the API and does not mutate the local store. Set `SAAS_ALERTS_NO_AUTO_REFRESH=1` to skip the freshness hook without changing source selection.

Covered paths:

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

When JSON output uses the generated provenance envelope, freshness metadata appears at `meta.freshness`. Treat it as current-cache freshness for the covered command path, not a guarantee of complete historical backfill or API-specific enrichment.

### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
saas-alerts-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

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

## Auth Setup

Create the key in SaaS Alerts under Settings > API (Show key under Manage API) and export it as SAAS_ALERTS_API_KEY. The same key works for the Reports, Manage, and Webhooks APIs. Respond calls need an extra access token: the CLI exchanges your key for one via POST /respondApi/api/v1/auth on first use, sends it as the accesstoken header, and re-mints it once if Respond rejects it. If every call returns 401 Access denied, ask SaaS Alerts support to enable the REST API for your account. EU-instance partners can point SAAS_ALERTS_BASE_URL at their host.

Run `saas-alerts-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, sync, and `--deliver` paths:

- `--json`  -  one JSON document on stdout (sync progress events go to stderr)
- `--compact`  -  keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain`  -  tabular rows (collection envelopes unwrap to the row array)
- `--quiet`  -  one identity value per row, no envelope

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  saas-alerts-cli alert-types --agent
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Writes** - generated create, update, and delete commands send immediately (add `--dry-run` to preview); `rule-triggers decide`, `contain`, and `subscriptions health --reenable` only preview until you pass `--confirm`
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

- Use `--home <dir>` for one invocation, or set `SAAS_ALERTS_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `SAAS_ALERTS_CONFIG_DIR`, `SAAS_ALERTS_DATA_DIR`, `SAAS_ALERTS_STATE_DIR`, `SAAS_ALERTS_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `SAAS_ALERTS_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `saas-alerts-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

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

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `SAAS_ALERTS_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `SAAS_ALERTS_HOME`, or `doctor` will not find credentials left under the former root.

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
saas-alerts-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "saas-alerts-cli learnings confirm 12"] }
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
       materially more, record the divergence via `saas-alerts-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `saas-alerts-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `saas-alerts-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
saas-alerts-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
saas-alerts-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
saas-alerts-cli teach-playbook \
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
saas-alerts-cli playbook amend \
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

`saas-alerts-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `SAAS_ALERTS_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
saas-alerts-cli feedback "the --since flag is inclusive but docs say exclusive"
saas-alerts-cli feedback --stdin < notes.txt
saas-alerts-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `SAAS_ALERTS_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `SAAS_ALERTS_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
saas-alerts-cli profile save briefing --json
saas-alerts-cli --profile briefing alert-types
saas-alerts-cli profile list --json
saas-alerts-cli profile show briefing
saas-alerts-cli profile delete briefing --yes
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
| 6 | Partial failure |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `saas-alerts-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add saas-alerts-mcp -- saas-alerts-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which saas-alerts-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   saas-alerts-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `saas-alerts-cli <command> --help`.
