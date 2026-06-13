# HaloPSA CLI

**Every HaloPSA, HaloITSM and HaloCRM feature, plus a local SQLite store and cross-entity views the API can't return.**

Wraps the full Halo REST API (952 endpoints across tickets, clients, assets, contracts, time, KB, and workflows) with offline-first search, agent-native JSON output, and cross-entity commands like `triage`, `client card`, and `contracts burn` that join tables Halo's UI scatters across five tabs.

## Install

The recommended path installs both the `halopsa-cli` binary and the `pp-halopsa` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install halopsa
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install halopsa --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install halopsa --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install halopsa --agent claude-code
npx -y @mvanhorn/printing-press-library install halopsa --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/project-management/halopsa/cmd/halopsa-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/halopsa-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install halopsa --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-halopsa --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-halopsa --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install halopsa --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/halopsa-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `HALOPSA_CLIENT_ID` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/project-management/halopsa/cmd/halopsa-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "halopsa": {
      "command": "halopsa-mcp",
      "env": {
        "HALOPSA_TENANT": "<tenant>",
        "HALOPSA_DOMAIN": "<domain>",
        "HALOPSA_CLIENT_ID": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Halo uses OAuth2 client_credentials. Create an API application in your tenant under Configuration > Integrations > Halo PSA API (Authentication Method: Client ID and Secret  -  Services), set `HALOPSA_TENANT=<yoursub>` in your env, then run `halopsa-cli auth login --client-id <id> --client-secret <secret>`. The CLI exchanges the credentials at https://<tenant>.halopsa.com/auth/token and caches the access token (auto-refreshed before expiry).

## Quick Start

```bash
# Confirm setup. Set HALOPSA_TENANT, HALOPSA_CLIENT_ID, and HALOPSA_CLIENT_SECRET in your env first (or run `halopsa-cli auth login` for an interactive prompt); doctor verifies the token mints and a sample GET succeeds.
halopsa-cli doctor

# First sync pulls tickets, clients, sites, agents, assets, contracts, KB into the local SQLite store
halopsa-cli sync --full

# The dispatcher view  -  per-agent load, stale count, 24h breach count in one table
halopsa-cli triage --team Support --json

# The keystone command  -  client + sites + tickets + contracts + assets + KB in one panel
halopsa-cli client card "Acme Corp" --json

# Drop into SQL when the prebuilt commands don't fit
halopsa-cli sql "SELECT status, COUNT(*) FROM tickets WHERE assigned_team='Support' GROUP BY status"

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Cross-entity dispatch views
- **`triage`**  -  See per-agent open ticket load, stale tickets, and 24-hour SLA-breach count in one table  -  the dispatcher view Halo's UI scatters across five tabs.

  _Reach for this when an agent asks 'who should I assign this P1 to' or 'where are we bleeding'. One call, one screen._

  ```bash
  halopsa-cli triage --team Support --json
  ```
- **`tickets age-out`**  -  Find tickets stale in a status for N days, preview them, then bulk-close with a templated action via --apply.

  _Reach for this on the Monday queue cleanse to close 30 stale tickets in one command instead of 30 clicks._

  ```bash
  halopsa-cli tickets age-out --status "Awaiting Customer" --stale-days 14 --action-note "Auto-closing per policy" --apply
  ```
- **`sla breaching`**  -  List tickets whose targetdate falls in the next N hours, sorted by time-to-breach, with agent + client + current status.

  _Reach for this on Friday afternoon or anytime before a hand-off to pre-empt SLA breaches._

  ```bash
  halopsa-cli sla breaching --within 24h --team Support --json
  ```
- **`agent workload`**  -  Per-agent: open tickets, tickets touched this week, billable hours logged, oldest open ticket age.

  _Reach for this when rebalancing the queue or asking 'who's overloaded'._

  ```bash
  halopsa-cli agent workload --team Support --json
  ```

### Per-client situational awareness
- **`client card`**  -  One panel: client + sites + active tickets + open contracts + contract hours remaining + recent KB articles linked to their tickets + asset count.

  _Reach for this on every client call. Open it before answering the phone, paste it into ticket notes._

  ```bash
  halopsa-cli client card "Acme Corp" --json
  ```
- **`asset history`**  -  Every ticket that touched this asset, chronological, with agent and time logged.

  _Reach for this when a machine keeps coming back to the queue  -  the pattern lives in the history, not in the latest ticket._

  ```bash
  halopsa-cli asset history LAP-0042 --json
  ```
- **`kbarticle suggest`**  -  FTS5-rank KB articles against a ticket's summary + details + last action text; print top 5 with snippets.

  _Reach for this mid-call when a known fix probably exists but you don't remember the exact KB title._

  ```bash
  halopsa-cli kbarticle suggest --ticket 12345 --limit 5
  ```

### Local-only analytics
- **`time gaps`**  -  List tickets the agent touched this week that have zero time logged on them.

  _Reach for this on Friday before submitting the timesheet. Stops 'I know I worked that ticket but where is it' archaeology._

  ```bash
  halopsa-cli time gaps --agent=me --week current
  ```
- **`contracts burn`**  -  Per contract: hours bank, hours consumed this period (sum of billable time on that client's tickets), days remaining, projected overage.

  _Reach for this mid-month before a client conversation  -  know whether they're tracking over their bank._

  ```bash
  halopsa-cli contracts burn --client "Acme Corp" --month current --json
  ```
- **`rules dump`**  -  Print every ticket rule and workflow as readable flat text  -  conditions → actions, one block per rule.

  _Reach for this during quarterly automation audits or when investigating an unexpected routing._

  ```bash
  halopsa-cli rules dump --workflow "New Ticket" > rules-audit.txt
  ```
- **`tickets changed-since`**  -  Tickets where any action or status change occurred since timestamp, grouped by ticket.

  _Reach for this after a meeting or when coming back from lunch. 'What did I miss' in one call._

  ```bash
  halopsa-cli tickets changed-since 09:00 --mine --json
  ```
- **`standup`**  -  Per-agent for the window: tickets closed, tickets reopened, time logged, top client.

  _Reach for this the moment before standup. One paste, everyone sees yesterday's progress._

  ```bash
  halopsa-cli standup --team Support --since yesterday
  ```
- **`client overlay`**  -  Rank all clients by a chosen metric (open tickets, stale, SLA at-risk, hours over bank). Top N out.

  _Reach for this when looking for the next escalation to invest time in. Whichever client is on top is the one to call._

  ```bash
  halopsa-cli client overlay --metric open_tickets --top 10 --json
  ```

### Local state that compounds
- **`time leaks`**  -  Billable time entries not yet attached to any invoice, summed by client and agent  -  the revenue sitting un-invoiced.

  _Reach for this in Monday billing prep to catch revenue leaks before invoices go out._

  ```bash
  halopsa-cli time leaks --month current --json
  ```

### Reporting that writes itself
- **`sla scorecard`**  -  Historical SLA pass-rate for closed tickets  -  % met resolution targets, by team or agent.

  _Reach for this for the weekly leadership report instead of a brittle export pipeline._

  ```bash
  halopsa-cli sla scorecard --since 30d --by team
  ```

### Cross-entity intelligence
- **`assets expiring`**  -  Assets whose linked contract ends in the next N days, joined to the owning client and sorted by days-to-expiry.

  _Reach for this in renewal prep and proactive replacement planning._

  ```bash
  halopsa-cli assets expiring --within 60
  ```
- **`tickets reopens`**  -  Tickets that bounced from closed back to open in the window, grouped by agent and client with reopen counts.

  _Reach for this in quality audits to find boomerang patterns by agent or client._

  ```bash
  halopsa-cli tickets reopens --since 30d
  ```

## Recipes


### Monday queue cleanse

```bash
halopsa-cli tickets age-out --status "Awaiting Customer Reply" --stale-days 14 --action-note "Auto-closing per policy"
```

Preview every stale customer-waiting ticket. Add --apply when you're ready to close them.

### Friday SLA radar before hand-off

```bash
halopsa-cli sla breaching --within 24h --team Support --agent --select id,summary,client_name,agent_name,minutes_to_breach
```

List every ticket the on-call shift inherits that's at risk of breaching SLA in their first 24h. Pipe to anything.

### Pre-call client briefing

```bash
halopsa-cli client card "Acme Corp" --agent --select active_tickets,contract_hours_remaining,assets,recent_kb_links
```

Get the client's complete situation in one query before answering the phone. Use --select to narrow what an agent sees.

### Friday timesheet reconcile

```bash
halopsa-cli time gaps --agent=me --week current --json
```

Find every ticket you touched this week with zero time logged so the gap doesn't ship with your timesheet.

### Contract overage check before client meeting

```bash
halopsa-cli contracts burn --client "Acme Corp" --month current --json
```

See current hours consumed vs. bank with projected overage so the contract conversation isn't a surprise.

## Usage

Run `halopsa-cli --help` for the full command reference and flag list.

## Commands

### actions

Manage actions

- **`halopsa-cli actions create`** - Create
- **`halopsa-cli actions create-reaction`** - Create reaction
- **`halopsa-cli actions create-review`** - Create review
- **`halopsa-cli actions delete`** - Delete
- **`halopsa-cli actions get`** - Use this to return a single instance of Actions.<br>
				Requires authentication.
- **`halopsa-cli actions list`** - Use this to return multiple Actions.<br>
				Requires authentication.

### addigy

Manage addigy

- **`halopsa-cli addigy create`** - Create
- **`halopsa-cli addigy list`** - List

### addigy-details

Manage addigy details

- **`halopsa-cli addigy-details create`** - Create
- **`halopsa-cli addigy-details delete`** - Delete
- **`halopsa-cli addigy-details get`** - Get
- **`halopsa-cli addigy-details list`** - List

### address

Manage address

- **`halopsa-cli address create`** - Create
- **`halopsa-cli address delete`** - Delete
- **`halopsa-cli address get`** - Use this to return a single instance of AddressStore.<br>
				Requires authentication.
- **`halopsa-cli address list`** - Use this to return multiple AddressStore.<br>
				Requires authentication.

### addressbook

Manage addressbook

- **`halopsa-cli addressbook create`** - Create
- **`halopsa-cli addressbook delete`** - Delete
- **`halopsa-cli addressbook get`** - Get
- **`halopsa-cli addressbook list`** - List

### adobe-acrobat-details

Manage adobe acrobat details

- **`halopsa-cli adobe-acrobat-details create`** - Create
- **`halopsa-cli adobe-acrobat-details delete`** - Delete
- **`halopsa-cli adobe-acrobat-details get`** - Get
- **`halopsa-cli adobe-acrobat-details list`** - List

### adobe-commerce-details

Manage adobe commerce details

- **`halopsa-cli adobe-commerce-details create`** - Create
- **`halopsa-cli adobe-commerce-details delete`** - Delete
- **`halopsa-cli adobe-commerce-details get`** - Get
- **`halopsa-cli adobe-commerce-details list`** - List

### adobe-commerce-integration

Manage adobe commerce integration

- **`halopsa-cli adobe-commerce-integration create`** - Create
- **`halopsa-cli adobe-commerce-integration list`** - List

### agent

Manage agent

- **`halopsa-cli agent create`** - Create
- **`halopsa-cli agent create-clearcache`** - Create clearcache
- **`halopsa-cli agent delete`** - Delete
- **`halopsa-cli agent get`** - Use this to return a single instance of Uname.<br>
				Requires authentication.
- **`halopsa-cli agent list`** - Use this to return multiple Uname.<br>
				Requires authentication.
- **`halopsa-cli agent list-me`** - List me

### agent-check-in

Manage agent check in

- **`halopsa-cli agent-check-in create`** - Create
- **`halopsa-cli agent-check-in get`** - Use this to return a single instance of AgentCheckIn.<br>
				Requires authentication.
- **`halopsa-cli agent-check-in list`** - Use this to return multiple AgentCheckIn.<br>
				Requires authentication.

### agent-event-subscription

Manage agent event subscription

- **`halopsa-cli agent-event-subscription create`** - Create
- **`halopsa-cli agent-event-subscription delete`** - Delete
- **`halopsa-cli agent-event-subscription get`** - Get
- **`halopsa-cli agent-event-subscription list`** - List

### agent-image

Manage agent image

- **`halopsa-cli agent-image <id>`** - Use this to return a single instance of Uname.<br>
				Requires authentication.

### agent-presence-rule

Manage agent presence rule

- **`halopsa-cli agent-presence-rule`** - List

### agent-presence-subscription

Manage agent presence subscription

- **`halopsa-cli agent-presence-subscription create`** - Create
- **`halopsa-cli agent-presence-subscription delete`** - Delete
- **`halopsa-cli agent-presence-subscription get-uname-presence-subscription`** - Get uname presence subscription
- **`halopsa-cli agent-presence-subscription list`** - List

### aisuggestion

Manage aisuggestion

- **`halopsa-cli aisuggestion create`** - Create
- **`halopsa-cli aisuggestion delete`** - Delete
- **`halopsa-cli aisuggestion get`** - Get
- **`halopsa-cli aisuggestion list`** - List

### alemba

Manage alemba

- **`halopsa-cli alemba`** - List

### amazon-seller-details

Manage amazon seller details

- **`halopsa-cli amazon-seller-details create`** - Create
- **`halopsa-cli amazon-seller-details delete`** - Delete
- **`halopsa-cli amazon-seller-details get`** - Get
- **`halopsa-cli amazon-seller-details list`** - List

### application

Manage application

- **`halopsa-cli application create`** - Create
- **`halopsa-cli application create-federatedcredentials`** - Create federatedcredentials
- **`halopsa-cli application delete`** - Delete
- **`halopsa-cli application get`** - Use this to return a single instance of NHD_Identity_Application.<br>
				Requires authentication.
- **`halopsa-cli application list`** - List

### appointment

Manage appointment

- **`halopsa-cli appointment create`** - Create
- **`halopsa-cli appointment create-booking`** - Create booking
- **`halopsa-cli appointment create-generate`** - Create generate
- **`halopsa-cli appointment delete`** - Delete specific Appointment.<br>
				Requires authentication.
- **`halopsa-cli appointment get`** - Use this to return a single instance of Appointment.<br>
				Requires authentication.
- **`halopsa-cli appointment list`** - Use this to return multiple Appointment.<br>
				Requires authentication.
- **`halopsa-cli appointment list-booking`** - List booking

### approval-process

Manage approval process

- **`halopsa-cli approval-process create`** - Create
- **`halopsa-cli approval-process delete`** - Delete
- **`halopsa-cli approval-process get`** - Use this to return a single instance of ApprovalProcess.<br>
				Requires authentication.
- **`halopsa-cli approval-process list`** - Use this to return multiple ApprovalProcess.<br>
				Requires authentication.

### approval-process-rule

Manage approval process rule

- **`halopsa-cli approval-process-rule create`** - Create
- **`halopsa-cli approval-process-rule delete`** - Delete
- **`halopsa-cli approval-process-rule get`** - Use this to return a single instance of ApprovalProcessRule.<br>
				Requires authentication.
- **`halopsa-cli approval-process-rule list`** - Use this to return multiple ApprovalProcessRule.<br>
				Requires authentication.

### area-azure-tenant

Manage area azure tenant

- **`halopsa-cli area-azure-tenant`** - Use this to return multiple AreaAzureTenant.<br>
				Requires authentication.

### area-request-type

Manage area request type

- **`halopsa-cli area-request-type get`** - Use this to return a single instance of AreaRequestType.<br>
				Requires authentication.
- **`halopsa-cli area-request-type list`** - List

### armis

Manage armis

- **`halopsa-cli armis`** - List

### armis-details

Manage armis details

- **`halopsa-cli armis-details create`** - Create
- **`halopsa-cli armis-details delete`** - Delete
- **`halopsa-cli armis-details get`** - Get
- **`halopsa-cli armis-details list`** - List

### arrow-sphere-details

Manage arrow sphere details

- **`halopsa-cli arrow-sphere-details create`** - Create
- **`halopsa-cli arrow-sphere-details delete`** - Delete
- **`halopsa-cli arrow-sphere-details get`** - Get
- **`halopsa-cli arrow-sphere-details list`** - List

### asset

Manage asset

- **`halopsa-cli asset create`** - Create
- **`halopsa-cli asset delete`** - Delete
- **`halopsa-cli asset get`** - Use this to return a single instance of Device.<br>
				Requires authentication.
- **`halopsa-cli asset list`** - Use this to return multiple Device.<br>
				Requires authentication.
- **`halopsa-cli asset list-getallsoftwareversions`** - List getallsoftwareversions
- **`halopsa-cli asset list-nexttag`** - List nexttag

### asset-change

Manage asset change

- **`halopsa-cli asset-change create`** - Create
- **`halopsa-cli asset-change list`** - Use this to return multiple DeviceChange.<br>
				Requires authentication.

### asset-group

Manage asset group

- **`halopsa-cli asset-group create`** - Create
- **`halopsa-cli asset-group delete`** - Delete
- **`halopsa-cli asset-group get`** - Use this to return a single instance of Generic.<br>
				Requires authentication.
- **`halopsa-cli asset-group list`** - Use this to return multiple Generic.<br>
				Requires authentication.

### asset-software

Manage asset software

- **`halopsa-cli asset-software`** - Use this to return multiple DeviceApplications.<br>
				Requires authentication.

### asset-type

Manage asset type

- **`halopsa-cli asset-type create`** - Create
- **`halopsa-cli asset-type delete`** - Delete
- **`halopsa-cli asset-type get`** - Use this to return a single instance of Xtype.<br>
				Requires authentication.
- **`halopsa-cli asset-type list`** - Use this to return multiple Xtype.<br>
				Requires authentication.

### asset-type-info

Manage asset type info

- **`halopsa-cli asset-type-info`** - Use this to return multiple Xtype.<br>
				Requires authentication.

### asset-type-mappings

Manage asset type mappings

- **`halopsa-cli asset-type-mappings get`** - Use this to return a single instance of XTypeMapping.<br>
				Requires authentication.
- **`halopsa-cli asset-type-mappings list`** - List

### att

Manage att

- **`halopsa-cli att`** - List

### attachment

Manage attachment

- **`halopsa-cli attachment create`** - Create
- **`halopsa-cli attachment create-document`** - Create document
- **`halopsa-cli attachment create-gets3presignedurl`** - Create gets3presignedurl
- **`halopsa-cli attachment create-image`** - Create image
- **`halopsa-cli attachment create-presignedurluploadcomplete`** - Create presignedurluploadcomplete
- **`halopsa-cli attachment delete`** - Delete
- **`halopsa-cli attachment delete-document`** - Delete document
- **`halopsa-cli attachment delete-image`** - Delete image
- **`halopsa-cli attachment get`** - Use this to return a single instance of Attachment.<br>
				Requires authentication.
- **`halopsa-cli attachment get-document`** - Get document
- **`halopsa-cli attachment get-image`** - Get image
- **`halopsa-cli attachment get-nhserver`** - Get nhserver
- **`halopsa-cli attachment list`** - Use this to return multiple Attachment.<br>
				Requires authentication.
- **`halopsa-cli attachment list-image`** - List image

### audit

Manage audit

- **`halopsa-cli audit create`** - Create
- **`halopsa-cli audit delete`** - Delete
- **`halopsa-cli audit get`** - Use this to return a single instance of Audit.<br>
				Requires authentication.
- **`halopsa-cli audit list`** - List

### auth-info

Manage auth info

- **`halopsa-cli auth-info`** - List

### automation

Manage automation

- **`halopsa-cli automation create`** - Create
- **`halopsa-cli automation create-runbookid`** - Create runbookid
- **`halopsa-cli automation delete`** - Delete
- **`halopsa-cli automation get`** - Get
- **`halopsa-cli automation list`** - List

### avalara-details

Manage avalara details

- **`halopsa-cli avalara-details create`** - Create
- **`halopsa-cli avalara-details delete`** - Delete
- **`halopsa-cli avalara-details get`** - Get
- **`halopsa-cli avalara-details list`** - List

### aws

Manage aws

- **`halopsa-cli aws`** - List

### awsdetails

Manage awsdetails

- **`halopsa-cli awsdetails create`** - Create
- **`halopsa-cli awsdetails delete`** - Delete
- **`halopsa-cli awsdetails get`** - Get
- **`halopsa-cli awsdetails list`** - List

### azure-delta

Manage azure delta

- **`halopsa-cli azure-delta create`** - Create
- **`halopsa-cli azure-delta delete`** - Delete
- **`halopsa-cli azure-delta get`** - Get
- **`halopsa-cli azure-delta list`** - List

### azure-dev-ops-details

Manage azure dev ops details

- **`halopsa-cli azure-dev-ops-details create`** - Create
- **`halopsa-cli azure-dev-ops-details delete`** - Delete
- **`halopsa-cli azure-dev-ops-details get`** - Use this to return a single instance of AzureDevOpsDetails.<br>
				Requires authentication.
- **`halopsa-cli azure-dev-ops-details list`** - List

### azure-translate

Manage azure translate

- **`halopsa-cli azure-translate create`** - Create
- **`halopsa-cli azure-translate list`** - List

### azureadconnection

Manage azureadconnection

- **`halopsa-cli azureadconnection create`** - Create
- **`halopsa-cli azureadconnection delete`** - Delete
- **`halopsa-cli azureadconnection get`** - Use this to return a single instance of AzureADConnection.<br>
				Requires authentication.
- **`halopsa-cli azureadconnection list`** - Use this to return multiple AzureADConnection.<br>
				Requires authentication.

### azureadmapping

Manage azureadmapping

- **`halopsa-cli azureadmapping`** - Use this to return multiple AzureADMapping.<br>
				Requires authentication.

### background-task

Manage background task

- **`halopsa-cli background-task <id>`** - Get

### billing-template

Manage billing template

- **`halopsa-cli billing-template create`** - Create
- **`halopsa-cli billing-template delete`** - Delete
- **`halopsa-cli billing-template get`** - Use this to return a single instance of ContractTemplateHeader.<br>
				Requires authentication.
- **`halopsa-cli billing-template list`** - List

### booking-type

Manage booking type

- **`halopsa-cli booking-type`** - Use this to return multiple BookingType.<br>
				Requires authentication.

### bookmark

Manage bookmark

- **`halopsa-cli bookmark create`** - Create
- **`halopsa-cli bookmark get`** - Get

### budget-type

Manage budget type

- **`halopsa-cli budget-type create`** - Create
- **`halopsa-cli budget-type delete`** - Delete
- **`halopsa-cli budget-type get`** - Use this to return a single instance of BudgetType.<br>
				Requires authentication.
- **`halopsa-cli budget-type list`** - Use this to return multiple BudgetType.<br>
				Requires authentication.

### bulk-email

Manage bulk email

- **`halopsa-cli bulk-email get`** - Use this to return a single instance of BulkEmail.<br>
				Requires authentication.
- **`halopsa-cli bulk-email list`** - List

### business-central-details

Manage business central details

- **`halopsa-cli business-central-details create`** - Create
- **`halopsa-cli business-central-details delete`** - Delete
- **`halopsa-cli business-central-details get`** - Use this to return a single instance of BusinessCentralDetails.<br>
				Requires authentication.
- **`halopsa-cli business-central-details list`** - Use this to return multiple BusinessCentralDetails.<br>
				Requires authentication.

### cab

Manage cab

- **`halopsa-cli cab create`** - Create
- **`halopsa-cli cab delete`** - Delete
- **`halopsa-cli cab get`** - Use this to return a single instance of CabHeader.<br>
				Requires authentication.
- **`halopsa-cli cab list`** - Use this to return multiple CabHeader.<br>
				Requires authentication.

### cabmember

Manage cabmember

- **`halopsa-cli cabmember`** - List

### cabrole

Manage cabrole

- **`halopsa-cli cabrole`** - List

### call-log

Manage call log

- **`halopsa-cli call-log create`** - Create
- **`halopsa-cli call-log get`** - Use this to return a single instance of CallLog.<br>
				Requires authentication.
- **`halopsa-cli call-log list`** - Use this to return multiple CallLog.<br>
				Requires authentication.

### call-script

Manage call script

- **`halopsa-cli call-script create`** - Create
- **`halopsa-cli call-script delete`** - Delete
- **`halopsa-cli call-script get`** - Use this to return a single instance of ScriptHeader.<br>
				Requires authentication.
- **`halopsa-cli call-script list`** - List

### canned-text

Manage canned text

- **`halopsa-cli canned-text create`** - Create
- **`halopsa-cli canned-text create-cannedtext`** - Create cannedtext
- **`halopsa-cli canned-text delete`** - Delete
- **`halopsa-cli canned-text get`** - Use this to return a single instance of CannedText.<br>
				Requires authentication.
- **`halopsa-cli canned-text list`** - Use this to return multiple CannedText.<br>
				Requires authentication.

### category

Manage category

- **`halopsa-cli category create`** - Create
- **`halopsa-cli category delete`** - Delete
- **`halopsa-cli category get`** - Use this to return a single instance of CategoryDetail.<br>
				Requires authentication.
- **`halopsa-cli category list`** - Use this to return multiple CategoryDetail.<br>
				Requires authentication.

### certificate

Manage certificate

- **`halopsa-cli certificate create`** - Create
- **`halopsa-cli certificate delete`** - Delete
- **`halopsa-cli certificate get`** - Use this to return a single instance of Certificate.<br>
				Requires authentication.
- **`halopsa-cli certificate list`** - List

### change-calendar

Manage change calendar

- **`halopsa-cli change-calendar`** - List

### charge-rate

Manage charge rate

- **`halopsa-cli charge-rate get`** - Use this to return a single instance of ChargeRate.<br>
				Requires authentication.
- **`halopsa-cli charge-rate list`** - Use this to return multiple ChargeRate.<br>
				Requires authentication.

### chat

Manage chat

- **`halopsa-cli chat create`** - Create
- **`halopsa-cli chat get`** - Get
- **`halopsa-cli chat list`** - Use this to return multiple LiveChatHeader.<br>
				Requires authentication.

### chat-flow

Manage chat flow

- **`halopsa-cli chat-flow`** - Create

### chat-matching-data

Manage chat matching data

- **`halopsa-cli chat-matching-data`** - Create

### chat-message

Manage chat message

- **`halopsa-cli chat-message create`** - Create
- **`halopsa-cli chat-message create-chatmessage`** - Create chatmessage
- **`halopsa-cli chat-message list`** - Use this to return multiple LiveChatMsg.<br>
				Requires authentication.

### chat-profile

Manage chat profile

- **`halopsa-cli chat-profile create`** - Create
- **`halopsa-cli chat-profile delete`** - Delete
- **`halopsa-cli chat-profile get`** - Use this to return a single instance of ChatProfile.<br>
				Requires authentication.
- **`halopsa-cli chat-profile list`** - Use this to return multiple ChatProfile.<br>
				Requires authentication.

### client-cache

Manage client cache

- **`halopsa-cli client-cache`** - List

### client-contract

Manage client contract

- **`halopsa-cli client-contract create`** - Create
- **`halopsa-cli client-contract create-clientcontract`** - Create clientcontract
- **`halopsa-cli client-contract create-clientcontract-2`** - Create clientcontract 2
- **`halopsa-cli client-contract delete`** - Delete
- **`halopsa-cli client-contract get`** - Use this to return a single instance of ContractHeader.<br>
				Requires authentication.
- **`halopsa-cli client-contract list`** - Use this to return multiple ContractHeader.<br>
				Requires authentication.

### client-prepay

Manage client prepay

- **`halopsa-cli client-prepay create`** - Create
- **`halopsa-cli client-prepay delete`** - Delete
- **`halopsa-cli client-prepay get`** - Use this to return a single instance of PrepayHistory.<br>
				Requires authentication.
- **`halopsa-cli client-prepay list`** - Use this to return multiple PrepayHistory.<br>
				Requires authentication.

### clients

Manage clients

- **`halopsa-cli clients create`** - Create
- **`halopsa-cli clients create-client`** - Create client
- **`halopsa-cli clients create-client-2`** - Create client 2
- **`halopsa-cli clients delete`** - Delete
- **`halopsa-cli clients get`** - Use this to return a single instance of Area.<br>
				Requires authentication.
- **`halopsa-cli clients list`** - Use this to return multiple Area.<br>
				Requires authentication.
- **`halopsa-cli clients list-client`** - List client

### config-commit

Manage config commit

- **`halopsa-cli config-commit create`** - Create
- **`halopsa-cli config-commit delete`** - Delete
- **`halopsa-cli config-commit get`** - Use this to return a single instance of ConfigCommit.<br>
				Requires authentication.
- **`halopsa-cli config-commit list`** - Use this to return multiple ConfigCommit.<br>
				Requires authentication.

### confirm-closure

Manage confirm closure

- **`halopsa-cli confirm-closure create`** - Create
- **`halopsa-cli confirm-closure delete`** - Delete
- **`halopsa-cli confirm-closure get`** - Use this to return a single instance of ConfirmClosure.<br>
				Requires authentication.
- **`halopsa-cli confirm-closure list`** - List

### confluence-details

Manage confluence details

- **`halopsa-cli confluence-details create`** - Create
- **`halopsa-cli confluence-details delete`** - Delete
- **`halopsa-cli confluence-details get`** - Get
- **`halopsa-cli confluence-details list`** - List

### connected-instance

Manage connected instance

- **`halopsa-cli connected-instance create`** - Create
- **`halopsa-cli connected-instance delete`** - Delete
- **`halopsa-cli connected-instance get`** - Use this to return a single instance of ConnectedInstance.<br>
				Requires authentication.
- **`halopsa-cli connected-instance list`** - List

### consignment

Manage consignment

- **`halopsa-cli consignment create`** - Create
- **`halopsa-cli consignment delete`** - Delete
- **`halopsa-cli consignment get`** - Use this to return a single instance of ConsignmentHeader.<br>
				Requires authentication.
- **`halopsa-cli consignment list`** - Use this to return multiple ConsignmentHeader.<br>
				Requires authentication.

### contactgroup

Manage contactgroup

- **`halopsa-cli contactgroup create`** - Create
- **`halopsa-cli contactgroup delete`** - Delete
- **`halopsa-cli contactgroup get`** - Get
- **`halopsa-cli contactgroup list`** - List

### contactgroupcontact

Manage contactgroupcontact

- **`halopsa-cli contactgroupcontact create`** - Create
- **`halopsa-cli contactgroupcontact delete`** - Delete
- **`halopsa-cli contactgroupcontact get`** - Get
- **`halopsa-cli contactgroupcontact list`** - List

### contract-rule

Manage contract rule

- **`halopsa-cli contract-rule create`** - Create
- **`halopsa-cli contract-rule delete`** - Delete
- **`halopsa-cli contract-rule get`** - Get
- **`halopsa-cli contract-rule list`** - List

### contract-schedule

Manage contract schedule

- **`halopsa-cli contract-schedule create`** - Create
- **`halopsa-cli contract-schedule delete`** - Delete
- **`halopsa-cli contract-schedule get`** - Use this to return a single instance of ContractSchedule.<br>
				Requires authentication.
- **`halopsa-cli contract-schedule list`** - List

### contract-schedule-plan

Manage contract schedule plan

- **`halopsa-cli contract-schedule-plan create`** - Create
- **`halopsa-cli contract-schedule-plan delete`** - Delete
- **`halopsa-cli contract-schedule-plan get`** - Use this to return a single instance of ContractSchedulePlan.<br>
				Requires authentication.
- **`halopsa-cli contract-schedule-plan list`** - List

### cost-centres

Manage cost centres

- **`halopsa-cli cost-centres create`** - Create
- **`halopsa-cli cost-centres delete`** - Delete
- **`halopsa-cli cost-centres get`** - Use this to return a single instance of Costcentres.<br>
				Requires authentication.
- **`halopsa-cli cost-centres list`** - List

### criteria-group

Manage criteria group

- **`halopsa-cli criteria-group`** - List

### crmnote

Manage crmnote

- **`halopsa-cli crmnote create`** - Create
- **`halopsa-cli crmnote delete`** - Delete
- **`halopsa-cli crmnote get`** - Use this to return a single instance of AreaNote.<br>
				Requires authentication.
- **`halopsa-cli crmnote list`** - Use this to return multiple AreaNote.<br>
				Requires authentication.

### cspconsumption-data

Manage cspconsumption data

- **`halopsa-cli cspconsumption-data create`** - Create
- **`halopsa-cli cspconsumption-data create-cspconsumptiondata`** - Create cspconsumptiondata
- **`halopsa-cli cspconsumption-data delete`** - Delete
- **`halopsa-cli cspconsumption-data delete-cspconsumptiondata`** - Delete cspconsumptiondata
- **`halopsa-cli cspconsumption-data get`** - Get
- **`halopsa-cli cspconsumption-data list`** - List

### cspinvoice

Manage cspinvoice

- **`halopsa-cli cspinvoice create`** - Create
- **`halopsa-cli cspinvoice delete`** - Delete
- **`halopsa-cli cspinvoice get`** - Get
- **`halopsa-cli cspinvoice list`** - List

### cspsubscription-pricing

Manage cspsubscription pricing

- **`halopsa-cli cspsubscription-pricing`** - Create

### csvtemplate

Manage csvtemplate

- **`halopsa-cli csvtemplate create`** - Create
- **`halopsa-cli csvtemplate delete`** - Delete
- **`halopsa-cli csvtemplate get`** - Use this to return a single instance of CSVTemplate.<br>
				Requires authentication.
- **`halopsa-cli csvtemplate list`** - List

### currency

Manage currency

- **`halopsa-cli currency create`** - Create
- **`halopsa-cli currency delete`** - Delete
- **`halopsa-cli currency get`** - Use this to return a single instance of Currency.<br>
				Requires authentication.
- **`halopsa-cli currency list`** - List

### custom-button

Manage custom button

- **`halopsa-cli custom-button create`** - Create
- **`halopsa-cli custom-button delete`** - Delete
- **`halopsa-cli custom-button get`** - Use this to return a single instance of CustomButton.<br>
				Requires authentication.
- **`halopsa-cli custom-button list`** - Use this to return multiple CustomButton.<br>
				Requires authentication.

### custom-button-audit

Manage custom button audit

- **`halopsa-cli custom-button-audit`** - Create

### custom-integration

Manage custom integration

- **`halopsa-cli custom-integration create`** - Create
- **`halopsa-cli custom-integration delete`** - Delete
- **`halopsa-cli custom-integration get`** - Use this to return a single instance of OutboundIntegration.<br>
				Requires authentication.
- **`halopsa-cli custom-integration list`** - List

### custom-integration-method

Manage custom integration method

- **`halopsa-cli custom-integration-method create`** - Create
- **`halopsa-cli custom-integration-method delete`** - Delete
- **`halopsa-cli custom-integration-method get`** - Use this to return a single instance of OutboundIntegrationMethod.<br>
				Requires authentication.
- **`halopsa-cli custom-integration-method list`** - Use this to return multiple OutboundIntegrationMethod.<br>
				Requires authentication.

### custom-integration-method-value

Manage custom integration method value

- **`halopsa-cli custom-integration-method-value`** - List

### custom-integration-repository

Manage custom integration repository

- **`halopsa-cli custom-integration-repository get`** - Use this to return a single instance of OutboundIntegration.<br>
				Requires authentication.
- **`halopsa-cli custom-integration-repository list`** - List

### custom-query

Manage custom query

- **`halopsa-cli custom-query create`** - Create
- **`halopsa-cli custom-query delete`** - Delete
- **`halopsa-cli custom-query get`** - Get
- **`halopsa-cli custom-query list`** - List

### custom-table

Manage custom table

- **`halopsa-cli custom-table create`** - Create
- **`halopsa-cli custom-table delete`** - Delete
- **`halopsa-cli custom-table get`** - Use this to return a single instance of CustomTable.<br>
				Requires authentication.
- **`halopsa-cli custom-table list`** - Use this to return multiple CustomTable.<br>
				Requires authentication.

### dashboard-links

Manage dashboard links

- **`halopsa-cli dashboard-links create`** - Create
- **`halopsa-cli dashboard-links delete`** - Delete
- **`halopsa-cli dashboard-links get`** - Use this to return a single instance of DashboardLinks.<br>
				Requires authentication.
- **`halopsa-cli dashboard-links list`** - Use this to return multiple DashboardLinks.<br>
				Requires authentication.
- **`halopsa-cli dashboard-links list-dashboardlinks`** - List dashboardlinks

### dashboard-links-repository

Manage dashboard links repository

- **`halopsa-cli dashboard-links-repository get`** - Use this to return a single instance of DashboardLinks.<br>
				Requires authentication.
- **`halopsa-cli dashboard-links-repository list`** - Use this to return multiple DashboardLinks.<br>
				Requires authentication.

### database-lookup

Manage database lookup

- **`halopsa-cli database-lookup create`** - Create
- **`halopsa-cli database-lookup create-databaselookup`** - Create databaselookup
- **`halopsa-cli database-lookup delete`** - Delete
- **`halopsa-cli database-lookup get`** - Use this to return a single instance of PartsLookup.<br>
				Requires authentication.
- **`halopsa-cli database-lookup list`** - Use this to return multiple PartsLookup.<br>
				Requires authentication.

### database-lookup-confirmation

Manage database lookup confirmation

- **`halopsa-cli database-lookup-confirmation create`** - Create
- **`halopsa-cli database-lookup-confirmation get`** - Get

### datto-commerce-details

Manage datto commerce details

- **`halopsa-cli datto-commerce-details create`** - Create
- **`halopsa-cli datto-commerce-details delete`** - Delete
- **`halopsa-cli datto-commerce-details get`** - Use this to return a single instance of DattoCommerceDetails.<br>
				Requires authentication.
- **`halopsa-cli datto-commerce-details list`** - Use this to return multiple DattoCommerceDetails.<br>
				Requires authentication.

### datto-rmm-details

Manage datto rmm details

- **`halopsa-cli datto-rmm-details create`** - Create
- **`halopsa-cli datto-rmm-details delete`** - Delete
- **`halopsa-cli datto-rmm-details get`** - Get
- **`halopsa-cli datto-rmm-details list`** - List

### device-licence

Manage device licence

- **`halopsa-cli device-licence`** - List

### distribution-lists

Manage distribution lists

- **`halopsa-cli distribution-lists create`** - Create
- **`halopsa-cli distribution-lists delete`** - Delete
- **`halopsa-cli distribution-lists get`** - Get
- **`halopsa-cli distribution-lists list`** - List

### distribution-lists-log

Manage distribution lists log

- **`halopsa-cli distribution-lists-log create`** - Create
- **`halopsa-cli distribution-lists-log delete`** - Delete
- **`halopsa-cli distribution-lists-log get`** - Get
- **`halopsa-cli distribution-lists-log list`** - List

### document-creation

Manage document creation

- **`halopsa-cli document-creation`** - Create

### downtime

Manage downtime

- **`halopsa-cli downtime create`** - Create
- **`halopsa-cli downtime delete`** - Delete
- **`halopsa-cli downtime get`** - Get
- **`halopsa-cli downtime list`** - List
- **`halopsa-cli downtime list-downtimecalendar`** - List downtimecalendar

### draft

Manage draft

- **`halopsa-cli draft`** - Create

### dynamics365-crmdetails

Manage dynamics365 crmdetails

- **`halopsa-cli dynamics365-crmdetails create`** - Create
- **`halopsa-cli dynamics365-crmdetails delete`** - Delete
- **`halopsa-cli dynamics365-crmdetails get`** - Get
- **`halopsa-cli dynamics365-crmdetails list`** - List

### dynatrace-details

Manage dynatrace details

- **`halopsa-cli dynatrace-details create`** - Create
- **`halopsa-cli dynatrace-details delete`** - Delete
- **`halopsa-cli dynatrace-details get`** - Get
- **`halopsa-cli dynatrace-details list`** - List

### ecommerce-order

Manage ecommerce order

- **`halopsa-cli ecommerce-order create`** - Create
- **`halopsa-cli ecommerce-order delete`** - Delete
- **`halopsa-cli ecommerce-order get`** - Get
- **`halopsa-cli ecommerce-order list`** - List

### email-address-book

Manage email address book

- **`halopsa-cli email-address-book`** - Use this to return multiple Users.<br>
				Requires authentication.

### email-rule

Manage email rule

- **`halopsa-cli email-rule create`** - Create
- **`halopsa-cli email-rule delete`** - Delete
- **`halopsa-cli email-rule get`** - Use this to return a single instance of EmailRule.<br>
				Requires authentication.
- **`halopsa-cli email-rule list`** - Use this to return multiple EmailRule.<br>
				Requires authentication.

### email-store

Manage email store

- **`halopsa-cli email-store create`** - Create
- **`halopsa-cli email-store delete`** - Delete
- **`halopsa-cli email-store get`** - Use this to return a single instance of EmailStore.<br>
				Requires authentication.
- **`halopsa-cli email-store list`** - List

### email-template

Manage email template

- **`halopsa-cli email-template create`** - Create
- **`halopsa-cli email-template create-emailtemplate`** - Create emailtemplate
- **`halopsa-cli email-template delete`** - Delete
- **`halopsa-cli email-template get`** - Use this to return a single instance of MessageContent.<br>
				Requires authentication.
- **`halopsa-cli email-template list`** - Use this to return multiple MessageContent.<br>
				Requires authentication.

### email-template-variable

Manage email template variable

- **`halopsa-cli email-template-variable create`** - Create
- **`halopsa-cli email-template-variable delete`** - Delete
- **`halopsa-cli email-template-variable get`** - Get
- **`halopsa-cli email-template-variable list`** - List

### eracent

Manage eracent

- **`halopsa-cli eracent`** - List

### eracent-details

Manage eracent details

- **`halopsa-cli eracent-details create`** - Create
- **`halopsa-cli eracent-details delete`** - Delete
- **`halopsa-cli eracent-details get`** - Get
- **`halopsa-cli eracent-details list`** - List

### event

Manage event

- **`halopsa-cli event create`** - Create
- **`halopsa-cli event delete`** - Delete
- **`halopsa-cli event get`** - Get
- **`halopsa-cli event list`** - List

### event-rule

Manage event rule

- **`halopsa-cli event-rule create`** - Create
- **`halopsa-cli event-rule delete`** - Delete
- **`halopsa-cli event-rule get`** - Get
- **`halopsa-cli event-rule list`** - List

### exact-details

Manage exact details

- **`halopsa-cli exact-details create`** - Create
- **`halopsa-cli exact-details delete`** - Delete
- **`halopsa-cli exact-details get`** - Use this to return a single instance of ExactDetails.<br>
				Requires authentication.
- **`halopsa-cli exact-details list`** - Use this to return multiple ExactDetails.<br>
				Requires authentication.

### example

Manage example

- **`halopsa-cli example`** - List

### expense

Manage expense

- **`halopsa-cli expense create`** - Create
- **`halopsa-cli expense list`** - List

### external-chat-message

Manage external chat message

- **`halopsa-cli external-chat-message create`** - Create
- **`halopsa-cli external-chat-message delete`** - Delete
- **`halopsa-cli external-chat-message get`** - Get
- **`halopsa-cli external-chat-message list`** - List

### external-link

Manage external link

- **`halopsa-cli external-link create`** - Create
- **`halopsa-cli external-link create-externallink`** - Create externallink
- **`halopsa-cli external-link delete`** - Delete
- **`halopsa-cli external-link get`** - Use this to return a single instance of ExternalLink.<br>
				Requires authentication.
- **`halopsa-cli external-link list`** - Use this to return multiple ExternalLink.<br>
				Requires authentication.

### facebook-details

Manage facebook details

- **`halopsa-cli facebook-details create`** - Create
- **`halopsa-cli facebook-details delete`** - Delete
- **`halopsa-cli facebook-details get`** - Use this to return a single instance of FacebookDetails.<br>
				Requires authentication.
- **`halopsa-cli facebook-details list`** - Use this to return multiple FacebookDetails.<br>
				Requires authentication.

### faqlists

Manage faqlists

- **`halopsa-cli faqlists create`** - Create
- **`halopsa-cli faqlists delete`** - Delete
- **`halopsa-cli faqlists get`** - Use this to return a single instance of FAQListHead.<br>
				Requires authentication.
- **`halopsa-cli faqlists list`** - Use this to return multiple FAQListHead.<br>
				Requires authentication.

### fault-view-log

Manage fault view log

- **`halopsa-cli fault-view-log`** - List

### faults-forecasting

Manage faults forecasting

- **`halopsa-cli faults-forecasting create`** - Create
- **`halopsa-cli faults-forecasting get`** - Use this to return a single instance of FaultsForecasting.<br>
				Requires authentication.

### features

Manage features

- **`halopsa-cli features create`** - Create
- **`halopsa-cli features get`** - Use this to return a single instance of ModuleSetup.<br>
				Requires authentication.
- **`halopsa-cli features list`** - Use this to return multiple ModuleSetup.<br>
				Requires authentication.

### feed

Manage feed

- **`halopsa-cli feed`** - Use this to return multiple Feed.<br>
				Requires authentication.

### feedback_items

Manage feedback items

- **`halopsa-cli feedback-items create`** - Create
- **`halopsa-cli feedback-items delete`** - Delete
- **`halopsa-cli feedback-items get`** - Use this to return a single instance of Feedback.<br>
				Requires authentication.
- **`halopsa-cli feedback-items list`** - List
- **`halopsa-cli feedback-items list-feedback`** - List feedback

### field

Manage field

- **`halopsa-cli field create`** - Create
- **`halopsa-cli field create-addfieldtoall`** - Create addfieldtoall
- **`halopsa-cli field delete`** - Delete specific Field.<br>
				Requires authentication.
- **`halopsa-cli field get`** - Use this to return a single instance of Field.<br>
				Requires authentication.
- **`halopsa-cli field list`** - Use this to return multiple Field.<br>
				Requires authentication.

### field-group

Manage field group

- **`halopsa-cli field-group create`** - Create
- **`halopsa-cli field-group delete`** - Delete
- **`halopsa-cli field-group get`** - Use this to return a single instance of FieldGroup.<br>
				Requires authentication.
- **`halopsa-cli field-group list`** - Use this to return multiple FieldGroup.<br>
				Requires authentication.

### field-info

Manage field info

- **`halopsa-cli field-info create`** - Create
- **`halopsa-cli field-info delete`** - Delete
- **`halopsa-cli field-info get`** - Use this to return a single instance of FieldInfo.<br>
				Requires authentication.
- **`halopsa-cli field-info list`** - Use this to return multiple FieldInfo.<br>
				Requires authentication.

### forecast-details

Manage forecast details

- **`halopsa-cli forecast-details create`** - Create
- **`halopsa-cli forecast-details delete`** - Delete
- **`halopsa-cli forecast-details get`** - Get
- **`halopsa-cli forecast-details list`** - List

### forethought-details

Manage forethought details

- **`halopsa-cli forethought-details create`** - Create
- **`halopsa-cli forethought-details delete`** - Delete
- **`halopsa-cli forethought-details get`** - Get
- **`halopsa-cli forethought-details list`** - List

### formattedemail

Manage formattedemail

- **`halopsa-cli formattedemail create`** - Create
- **`halopsa-cli formattedemail delete`** - Delete
- **`halopsa-cli formattedemail get`** - Use this to return a single instance of formattedemail.<br>
				Requires authentication.
- **`halopsa-cli formattedemail list`** - List

### fortnox-details

Manage fortnox details

- **`halopsa-cli fortnox-details create`** - Create
- **`halopsa-cli fortnox-details delete`** - Delete
- **`halopsa-cli fortnox-details get`** - Get
- **`halopsa-cli fortnox-details list`** - List

### go-to-resolve

Manage go to resolve

- **`halopsa-cli go-to-resolve list`** - List
- **`halopsa-cli go-to-resolve list-gotoresolve`** - List gotoresolve

### google-business-details

Manage google business details

- **`halopsa-cli google-business-details create`** - Create
- **`halopsa-cli google-business-details delete`** - Delete
- **`halopsa-cli google-business-details get`** - Get
- **`halopsa-cli google-business-details list`** - List

### gworkspace-details

Manage gworkspace details

- **`halopsa-cli gworkspace-details create`** - Create
- **`halopsa-cli gworkspace-details delete`** - Delete
- **`halopsa-cli gworkspace-details get`** - Get
- **`halopsa-cli gworkspace-details list`** - List

### halo-device-info

Manage halo device info

- **`halopsa-cli halo-device-info create`** - Create
- **`halopsa-cli halo-device-info delete`** - Delete
- **`halopsa-cli halo-device-info get`** - Get

### halo-field

Manage halo field

- **`halopsa-cli halo-field`** - List

### halo-integration

Manage halo integration

- **`halopsa-cli halo-integration create`** - Create
- **`halopsa-cli halo-integration create-halointegration`** - Create halointegration
- **`halopsa-cli halo-integration list`** - List

### halo-news

Manage halo news

- **`halopsa-cli halo-news create`** - Create
- **`halopsa-cli halo-news create-halonews`** - Create halonews
- **`halopsa-cli halo-news delete`** - Delete
- **`halopsa-cli halo-news get`** - Use this to return a single instance of HaloNews.<br>
				Requires authentication.
- **`halopsa-cli halo-news list`** - List

### halo_search

Manage halo search

- **`halopsa-cli halo-search`** - Use this to return multiple Search.<br>
				Requires authentication.

### health

Manage health

- **`halopsa-cli health list`** - List
- **`halopsa-cli health list-hashing`** - List hashing

### historical-ticket-volumes

Manage historical ticket volumes

- **`halopsa-cli historical-ticket-volumes create`** - Create
- **`halopsa-cli historical-ticket-volumes delete`** - Delete
- **`halopsa-cli historical-ticket-volumes get`** - Get
- **`halopsa-cli historical-ticket-volumes list`** - List

### holiday

Manage holiday

- **`halopsa-cli holiday create`** - Create
- **`halopsa-cli holiday delete`** - Delete
- **`halopsa-cli holiday get`** - Use this to return a single instance of Holidays.<br>
				Requires authentication.
- **`halopsa-cli holiday list`** - Use this to return multiple Holidays.<br>
				Requires authentication.

### hopewiser

Manage hopewiser

- **`halopsa-cli hopewiser`** - List

### impersonation-request

Manage impersonation request

- **`halopsa-cli impersonation-request`** - Create

### import-csv

Manage import csv

- **`halopsa-cli import-csv create`** - Create
- **`halopsa-cli import-csv delete`** - Delete
- **`halopsa-cli import-csv get`** - Use this to return a single instance of ImportCsv.<br>
				Requires authentication.
- **`halopsa-cli import-csv list`** - Use this to return multiple ImportCsv.<br>
				Requires authentication.

### incoming-event

Manage incoming event

- **`halopsa-cli incoming-event create`** - Create
- **`halopsa-cli incoming-event create-incomingevent`** - Create incomingevent
- **`halopsa-cli incoming-event delete`** - Delete
- **`halopsa-cli incoming-event get`** - Get
- **`halopsa-cli incoming-event list`** - List

### incoming-webhook

Manage incoming webhook

- **`halopsa-cli incoming-webhook create`** - Create
- **`halopsa-cli incoming-webhook create-incomingwebhook`** - Create incomingwebhook
- **`halopsa-cli incoming-webhook delete`** - Delete
- **`halopsa-cli incoming-webhook get`** - Get
- **`halopsa-cli incoming-webhook list`** - List

### incoming-webhook-attempt

Manage incoming webhook attempt

- **`halopsa-cli incoming-webhook-attempt`** - List

### incomingemail

Manage incomingemail

- **`halopsa-cli incomingemail create`** - Create
- **`halopsa-cli incomingemail create-addtoticket`** - Create addtoticket
- **`halopsa-cli incomingemail delete`** - Delete
- **`halopsa-cli incomingemail get`** - Use this to return a single instance of IncomingEmail.<br>
				Requires authentication.
- **`halopsa-cli incomingemail list`** - Use this to return multiple IncomingEmail.<br>
				Requires authentication.

### ingram-micro-details

Manage ingram micro details

- **`halopsa-cli ingram-micro-details create`** - Create
- **`halopsa-cli ingram-micro-details delete`** - Delete
- **`halopsa-cli ingram-micro-details get`** - Use this to return a single instance of IngramMicroDetails.<br>
				Requires authentication.
- **`halopsa-cli ingram-micro-details list`** - List

### ingram-micro-reseller

Manage ingram micro reseller

- **`halopsa-cli ingram-micro-reseller list`** - List
- **`halopsa-cli ingram-micro-reseller list-ingrammicroreseller`** - List ingrammicroreseller

### ingram-micro-reseller-details

Manage ingram micro reseller details

- **`halopsa-cli ingram-micro-reseller-details create`** - Create
- **`halopsa-cli ingram-micro-reseller-details delete`** - Delete
- **`halopsa-cli ingram-micro-reseller-details get`** - Get
- **`halopsa-cli ingram-micro-reseller-details list`** - List

### instance

Manage instance

- **`halopsa-cli instance create`** - Create
- **`halopsa-cli instance get`** - Get
- **`halopsa-cli instance list`** - Use this to return multiple Instance.<br>
				Requires authentication.

### instance-info

Manage instance info

- **`halopsa-cli instance-info`** - List

### integration-configuration

Manage integration configuration

- **`halopsa-cli integration-configuration create`** - Create
- **`halopsa-cli integration-configuration get`** - Use this to return a single instance of IntegrationConfiguration.<br>
				Requires authentication.
- **`halopsa-cli integration-configuration list`** - List

### integration-data

Manage integration data

- **`halopsa-cli integration-data create`** - Create
- **`halopsa-cli integration-data create-integrationdata`** - Create integrationdata
- **`halopsa-cli integration-data create-integrationdata-10`** - Create integrationdata 10
- **`halopsa-cli integration-data create-integrationdata-11`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data create-integrationdata-12`** - Create integrationdata 12
- **`halopsa-cli integration-data create-integrationdata-13`** - Create integrationdata 13
- **`halopsa-cli integration-data create-integrationdata-14`** - Create integrationdata 14
- **`halopsa-cli integration-data create-integrationdata-15`** - Create integrationdata 15
- **`halopsa-cli integration-data create-integrationdata-16`** - Create integrationdata 16
- **`halopsa-cli integration-data create-integrationdata-17`** - Create integrationdata 17
- **`halopsa-cli integration-data create-integrationdata-18`** - Create integrationdata 18
- **`halopsa-cli integration-data create-integrationdata-19`** - Create integrationdata 19
- **`halopsa-cli integration-data create-integrationdata-2`** - Create integrationdata 2
- **`halopsa-cli integration-data create-integrationdata-20`** - Create integrationdata 20
- **`halopsa-cli integration-data create-integrationdata-21`** - Create integrationdata 21
- **`halopsa-cli integration-data create-integrationdata-22`** - Create integrationdata 22
- **`halopsa-cli integration-data create-integrationdata-23`** - Create integrationdata 23
- **`halopsa-cli integration-data create-integrationdata-24`** - Create integrationdata 24
- **`halopsa-cli integration-data create-integrationdata-25`** - Create integrationdata 25
- **`halopsa-cli integration-data create-integrationdata-26`** - Create integrationdata 26
- **`halopsa-cli integration-data create-integrationdata-27`** - Create integrationdata 27
- **`halopsa-cli integration-data create-integrationdata-28`** - Create integrationdata 28
- **`halopsa-cli integration-data create-integrationdata-29`** - Create integrationdata 29
- **`halopsa-cli integration-data create-integrationdata-3`** - Create integrationdata 3
- **`halopsa-cli integration-data create-integrationdata-30`** - Create integrationdata 30
- **`halopsa-cli integration-data create-integrationdata-31`** - Create integrationdata 31
- **`halopsa-cli integration-data create-integrationdata-32`** - Create integrationdata 32
- **`halopsa-cli integration-data create-integrationdata-33`** - Create integrationdata 33
- **`halopsa-cli integration-data create-integrationdata-34`** - Create integrationdata 34
- **`halopsa-cli integration-data create-integrationdata-35`** - Create integrationdata 35
- **`halopsa-cli integration-data create-integrationdata-36`** - Create integrationdata 36
- **`halopsa-cli integration-data create-integrationdata-37`** - Create integrationdata 37
- **`halopsa-cli integration-data create-integrationdata-38`** - Create integrationdata 38
- **`halopsa-cli integration-data create-integrationdata-39`** - Create integrationdata 39
- **`halopsa-cli integration-data create-integrationdata-4`** - Create integrationdata 4
- **`halopsa-cli integration-data create-integrationdata-40`** - Create integrationdata 40
- **`halopsa-cli integration-data create-integrationdata-41`** - Create integrationdata 41
- **`halopsa-cli integration-data create-integrationdata-42`** - Create integrationdata 42
- **`halopsa-cli integration-data create-integrationdata-5`** - Create integrationdata 5
- **`halopsa-cli integration-data create-integrationdata-6`** - Create integrationdata 6
- **`halopsa-cli integration-data create-integrationdata-7`** - Create integrationdata 7
- **`halopsa-cli integration-data create-integrationdata-8`** - Create integrationdata 8
- **`halopsa-cli integration-data create-integrationdata-9`** - Create integrationdata 9
- **`halopsa-cli integration-data get`** - Get
- **`halopsa-cli integration-data list`** - List
- **`halopsa-cli integration-data list-integrationdata`** - List integrationdata
- **`halopsa-cli integration-data list-integrationdata-10`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-100`** - List integrationdata 100
- **`halopsa-cli integration-data list-integrationdata-101`** - List integrationdata 101
- **`halopsa-cli integration-data list-integrationdata-102`** - List integrationdata 102
- **`halopsa-cli integration-data list-integrationdata-103`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-104`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-105`** - List integrationdata 105
- **`halopsa-cli integration-data list-integrationdata-106`** - List integrationdata 106
- **`halopsa-cli integration-data list-integrationdata-107`** - List integrationdata 107
- **`halopsa-cli integration-data list-integrationdata-108`** - List integrationdata 108
- **`halopsa-cli integration-data list-integrationdata-109`** - List integrationdata 109
- **`halopsa-cli integration-data list-integrationdata-11`** - List integrationdata 11
- **`halopsa-cli integration-data list-integrationdata-110`** - List integrationdata 110
- **`halopsa-cli integration-data list-integrationdata-111`** - List integrationdata 111
- **`halopsa-cli integration-data list-integrationdata-112`** - List integrationdata 112
- **`halopsa-cli integration-data list-integrationdata-12`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-13`** - List integrationdata 13
- **`halopsa-cli integration-data list-integrationdata-14`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-15`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-16`** - List integrationdata 16
- **`halopsa-cli integration-data list-integrationdata-17`** - List integrationdata 17
- **`halopsa-cli integration-data list-integrationdata-18`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-19`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-2`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-20`** - List integrationdata 20
- **`halopsa-cli integration-data list-integrationdata-21`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-22`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-23`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-24`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-25`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-26`** - List integrationdata 26
- **`halopsa-cli integration-data list-integrationdata-27`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-28`** - List integrationdata 28
- **`halopsa-cli integration-data list-integrationdata-29`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-3`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-30`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-31`** - List integrationdata 31
- **`halopsa-cli integration-data list-integrationdata-32`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-33`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-34`** - List integrationdata 34
- **`halopsa-cli integration-data list-integrationdata-35`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-36`** - List integrationdata 36
- **`halopsa-cli integration-data list-integrationdata-37`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-38`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-39`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-4`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-40`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-41`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-42`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-43`** - List integrationdata 43
- **`halopsa-cli integration-data list-integrationdata-44`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-45`** - List integrationdata 45
- **`halopsa-cli integration-data list-integrationdata-46`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-47`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-48`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-49`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-5`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-50`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-51`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-52`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-53`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-54`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-55`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-56`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-57`** - List integrationdata 57
- **`halopsa-cli integration-data list-integrationdata-58`** - List integrationdata 58
- **`halopsa-cli integration-data list-integrationdata-59`** - List integrationdata 59
- **`halopsa-cli integration-data list-integrationdata-6`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-60`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-61`** - List integrationdata 61
- **`halopsa-cli integration-data list-integrationdata-62`** - List integrationdata 62
- **`halopsa-cli integration-data list-integrationdata-63`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-64`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-65`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-66`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-67`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-68`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-69`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-7`** - List integrationdata 7
- **`halopsa-cli integration-data list-integrationdata-70`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-71`** - List integrationdata 71
- **`halopsa-cli integration-data list-integrationdata-72`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-73`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-74`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-75`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-76`** - List integrationdata 76
- **`halopsa-cli integration-data list-integrationdata-77`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-78`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-79`** - List integrationdata 79
- **`halopsa-cli integration-data list-integrationdata-8`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-80`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-81`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-82`** - List integrationdata 82
- **`halopsa-cli integration-data list-integrationdata-83`** - List integrationdata 83
- **`halopsa-cli integration-data list-integrationdata-84`** - List integrationdata 84
- **`halopsa-cli integration-data list-integrationdata-85`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-86`** - List integrationdata 86
- **`halopsa-cli integration-data list-integrationdata-87`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-88`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-89`** - List integrationdata 89
- **`halopsa-cli integration-data list-integrationdata-9`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-90`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-91`** - List integrationdata 91
- **`halopsa-cli integration-data list-integrationdata-92`** - List integrationdata 92
- **`halopsa-cli integration-data list-integrationdata-93`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-94`** - List integrationdata 94
- **`halopsa-cli integration-data list-integrationdata-95`** - List integrationdata 95
- **`halopsa-cli integration-data list-integrationdata-96`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-97`** - .<br>
				Requires authentication.
- **`halopsa-cli integration-data list-integrationdata-98`** - List integrationdata 98
- **`halopsa-cli integration-data list-integrationdata-99`** - List integrationdata 99

### integration-delta

Manage integration delta

- **`halopsa-cli integration-delta create`** - Create
- **`halopsa-cli integration-delta delete`** - Delete
- **`halopsa-cli integration-delta get`** - Get
- **`halopsa-cli integration-delta list`** - List

### integration-error

Manage integration error

- **`halopsa-cli integration-error create`** - Create
- **`halopsa-cli integration-error delete`** - Delete
- **`halopsa-cli integration-error get`** - Use this to return a single instance of IntegrationError.<br>
				Requires authentication.
- **`halopsa-cli integration-error list`** - Use this to return multiple IntegrationError.<br>
				Requires authentication.

### integration-export

Manage integration export

- **`halopsa-cli integration-export create`** - Create
- **`halopsa-cli integration-export delete`** - Delete
- **`halopsa-cli integration-export list`** - Use this to return multiple IntegrationExport.<br>
				Requires authentication.

### integration-field-data

Manage integration field data

- **`halopsa-cli integration-field-data create`** - Create
- **`halopsa-cli integration-field-data delete`** - Delete
- **`halopsa-cli integration-field-data get`** - Get
- **`halopsa-cli integration-field-data list`** - List

### integration-field-mapping

Manage integration field mapping

- **`halopsa-cli integration-field-mapping`** - Use this to return multiple IntegrationFieldMapping.<br>
				Requires authentication.

### integration-look-up

Manage integration look up

- **`halopsa-cli integration-look-up create`** - Create
- **`halopsa-cli integration-look-up list`** - List

### integration-request

Manage integration request

- **`halopsa-cli integration-request create`** - Create
- **`halopsa-cli integration-request delete`** - Delete
- **`halopsa-cli integration-request get`** - Use this to return a single instance of IntegrationRequest.<br>
				Requires authentication.
- **`halopsa-cli integration-request list`** - Use this to return multiple IntegrationRequest.<br>
				Requires authentication.

### integration-runbook-variable-group

Manage integration runbook variable group

- **`halopsa-cli integration-runbook-variable-group get`** - Use this to return a single instance of IntegrationRunbookVariableGroup.<br>
				Requires authentication.
- **`halopsa-cli integration-runbook-variable-group list`** - Use this to return multiple IntegrationRunbookVariableGroup.<br>
				Requires authentication.

### integration-site-mapping

Manage integration site mapping

- **`halopsa-cli integration-site-mapping`** - Use this to return multiple IntegrationSiteMapping.<br>
				Requires authentication.

### integrator-log

Manage integrator log

- **`halopsa-cli integrator-log`** - Use this to return multiple IntegratorLog.<br>
				Requires authentication.

### integrator-schedule

Manage integrator schedule

- **`halopsa-cli integrator-schedule`** - Use this to return multiple IntegratorSchedule.<br>
				Requires authentication.

### integrator-trace

Manage integrator trace

- **`halopsa-cli integrator-trace get`** - Get
- **`halopsa-cli integrator-trace list`** - List

### invoice

Manage invoice

- **`halopsa-cli invoice create`** - Create
- **`halopsa-cli invoice create-pdf`** - Create pdf
- **`halopsa-cli invoice create-updatelines`** - Create updatelines
- **`halopsa-cli invoice create-view`** - Create view
- **`halopsa-cli invoice delete`** - Delete specific InvoiceHeader.<br>
				Requires authentication.
- **`halopsa-cli invoice get`** - Use this to return a single instance of InvoiceHeader.<br>
				Requires authentication.
- **`halopsa-cli invoice list`** - Use this to return multiple InvoiceHeader.<br>
				Requires authentication.
- **`halopsa-cli invoice list-lines`** - List lines

### invoice-change

Manage invoice change

- **`halopsa-cli invoice-change create`** - Create
- **`halopsa-cli invoice-change list`** - Use this to return multiple InvoiceChange.<br>
				Requires authentication.

### invoice-detail-pro-rata

Manage invoice detail pro rata

- **`halopsa-cli invoice-detail-pro-rata`** - List

### invoice-payment

Manage invoice payment

- **`halopsa-cli invoice-payment create`** - Create
- **`halopsa-cli invoice-payment delete`** - Delete
- **`halopsa-cli invoice-payment get`** - Use this to return a single instance of InvoicePayment.<br>
				Requires authentication.
- **`halopsa-cli invoice-payment list`** - Use this to return multiple InvoicePayment.<br>
				Requires authentication.

### islonline

Manage islonline

- **`halopsa-cli islonline create`** - Create
- **`halopsa-cli islonline list`** - List

### item

Manage item

- **`halopsa-cli item create`** - Create
- **`halopsa-cli item create-newaccountsid`** - Create newaccountsid
- **`halopsa-cli item delete`** - Delete
- **`halopsa-cli item get`** - Use this to return a single instance of Item.<br>
				Requires authentication.
- **`halopsa-cli item list`** - Use this to return multiple Item.<br>
				Requires authentication.

### item-accounts-link

Manage item accounts link

- **`halopsa-cli item-accounts-link create`** - Create
- **`halopsa-cli item-accounts-link create-itemaccountslink`** - Create itemaccountslink
- **`halopsa-cli item-accounts-link delete`** - Delete
- **`halopsa-cli item-accounts-link get`** - Get
- **`halopsa-cli item-accounts-link list`** - List

### item-group

Manage item group

- **`halopsa-cli item-group create`** - Create
- **`halopsa-cli item-group delete`** - Delete
- **`halopsa-cli item-group get`** - Use this to return a single instance of ItemGroup.<br>
				Requires authentication.
- **`halopsa-cli item-group list`** - List

### item-stock

Manage item stock

- **`halopsa-cli item-stock create`** - Create
- **`halopsa-cli item-stock delete`** - Delete
- **`halopsa-cli item-stock get`** - Use this to return a single instance of ItemStock.<br>
				Requires authentication.
- **`halopsa-cli item-stock list`** - Use this to return multiple ItemStock.<br>
				Requires authentication.

### item-stock-history

Manage item stock history

- **`halopsa-cli item-stock-history get`** - Get
- **`halopsa-cli item-stock-history list`** - Use this to return multiple ItemStockHistory.<br>
				Requires authentication.

### itemsupplier

Manage itemsupplier

- **`halopsa-cli itemsupplier create`** - Create
- **`halopsa-cli itemsupplier delete`** - Delete
- **`halopsa-cli itemsupplier get`** - Use this to return a single instance of ItemSupplier.<br>
				Requires authentication.
- **`halopsa-cli itemsupplier list`** - List

### jamf-details

Manage jamf details

- **`halopsa-cli jamf-details create`** - Create
- **`halopsa-cli jamf-details delete`** - Delete
- **`halopsa-cli jamf-details get`** - Get
- **`halopsa-cli jamf-details list`** - List

### jira-details

Manage jira details

- **`halopsa-cli jira-details create`** - Create
- **`halopsa-cli jira-details delete`** - Delete
- **`halopsa-cli jira-details get`** - Get
- **`halopsa-cli jira-details list`** - List

### journey

Manage journey

- **`halopsa-cli journey create`** - Create
- **`halopsa-cli journey delete`** - Delete
- **`halopsa-cli journey get`** - Use this to return a single instance of Journey.<br>
				Requires authentication.
- **`halopsa-cli journey list`** - List

### kandji

Manage kandji

- **`halopsa-cli kandji`** - List

### kandji-details

Manage kandji details

- **`halopsa-cli kandji-details create`** - Create
- **`halopsa-cli kandji-details delete`** - Delete
- **`halopsa-cli kandji-details get`** - Get
- **`halopsa-cli kandji-details list`** - List

### kaseya-vsax

Manage kaseya vsax

- **`halopsa-cli kaseya-vsax create`** - Create
- **`halopsa-cli kaseya-vsax delete`** - Delete
- **`halopsa-cli kaseya-vsax list`** - List

### kaseya-vsaxdetails

Manage kaseya vsaxdetails

- **`halopsa-cli kaseya-vsaxdetails create`** - Create
- **`halopsa-cli kaseya-vsaxdetails delete`** - Delete
- **`halopsa-cli kaseya-vsaxdetails get`** - Get
- **`halopsa-cli kaseya-vsaxdetails list`** - List

### kashflow-details

Manage kashflow details

- **`halopsa-cli kashflow-details create`** - Create
- **`halopsa-cli kashflow-details delete`** - Delete
- **`halopsa-cli kashflow-details get`** - Use this to return a single instance of KashflowDetails.<br>
				Requires authentication.
- **`halopsa-cli kashflow-details list`** - Use this to return multiple KashflowDetails.<br>
				Requires authentication.

### kbarticle

Manage kbarticle

- **`halopsa-cli kbarticle create`** - Create
- **`halopsa-cli kbarticle create-vote`** - Create vote
- **`halopsa-cli kbarticle delete`** - Delete
- **`halopsa-cli kbarticle get`** - Use this to return a single instance of KBEntry.<br>
				Requires authentication.
- **`halopsa-cli kbarticle list`** - Use this to return multiple KBEntry.<br>
				Requires authentication.

### kbarticle-anon

Manage kbarticle anon

- **`halopsa-cli kbarticle-anon get`** - Get
- **`halopsa-cli kbarticle-anon list`** - List

### key-vault

Manage key vault

- **`halopsa-cli key-vault create`** - Create
- **`halopsa-cli key-vault delete`** - Delete
- **`halopsa-cli key-vault get`** - Get
- **`halopsa-cli key-vault list`** - List

### languages

Manage languages

- **`halopsa-cli languages create`** - Create
- **`halopsa-cli languages delete`** - Delete
- **`halopsa-cli languages get`** - Use this to return a single instance of LanguagePack.<br>
				Requires authentication.
- **`halopsa-cli languages list`** - Use this to return multiple LanguagePack.<br>
				Requires authentication.

### lap-safe

Manage lap safe

- **`halopsa-cli lap-safe list`** - List
- **`halopsa-cli lap-safe list-lapsafe`** - List lapsafe
- **`halopsa-cli lap-safe list-lapsafe-2`** - List lapsafe 2

### ldapconnection

Manage ldapconnection

- **`halopsa-cli ldapconnection create`** - Create
- **`halopsa-cli ldapconnection delete`** - Delete
- **`halopsa-cli ldapconnection get`** - Use this to return a single instance of LDAPConnection.<br>
				Requires authentication.
- **`halopsa-cli ldapconnection list`** - Use this to return multiple LDAPConnection.<br>
				Requires authentication.

### licence-change

Manage licence change

- **`halopsa-cli licence-change`** - Use this to return multiple LicenceChange.<br>
				Requires authentication.

### license-info

Manage license info

- **`halopsa-cli license-info create`** - Create
- **`halopsa-cli license-info list`** - Use this to return multiple LicenceInfo.<br>
				Requires authentication.
- **`halopsa-cli license-info list-licenseinfo`** - List licenseinfo

### login-token

Manage login token

- **`halopsa-cli login-token`** - Create

### lookup

Manage lookup

- **`halopsa-cli lookup create`** - Create
- **`halopsa-cli lookup create-clearcache`** - Create clearcache
- **`halopsa-cli lookup delete`** - Delete
- **`halopsa-cli lookup get`** - Use this to return a single instance of Lookup.<br>
				Requires authentication.
- **`halopsa-cli lookup list`** - Use this to return multiple Lookup.<br>
				Requires authentication.

### mail

Manage mail

- **`halopsa-cli mail create`** - Create
- **`halopsa-cli mail create-integrator`** - Create integrator
- **`halopsa-cli mail create-integrator-2`** - Create integrator 2
- **`halopsa-cli mail create-integrator-3`** - Create integrator 3
- **`halopsa-cli mail create-integrator-4`** - Create integrator 4
- **`halopsa-cli mail create-processmail`** - Create processmail

### mail-campaign

Manage mail campaign

- **`halopsa-cli mail-campaign create`** - Create
- **`halopsa-cli mail-campaign delete`** - Delete
- **`halopsa-cli mail-campaign get`** - Get
- **`halopsa-cli mail-campaign list`** - List

### mail-campaign-email

Manage mail campaign email

- **`halopsa-cli mail-campaign-email create`** - Create
- **`halopsa-cli mail-campaign-email delete`** - Delete
- **`halopsa-cli mail-campaign-email get`** - Get
- **`halopsa-cli mail-campaign-email list`** - List

### mail-campaign-log

Manage mail campaign log

- **`halopsa-cli mail-campaign-log get`** - Get
- **`halopsa-cli mail-campaign-log list`** - List

### mailbox

Manage mailbox

- **`halopsa-cli mailbox create`** - Create
- **`halopsa-cli mailbox delete`** - Delete
- **`halopsa-cli mailbox get`** - Use this to return a single instance of Mailbox.<br>
				Requires authentication.
- **`halopsa-cli mailbox list`** - Use this to return multiple Mailbox.<br>
				Requires authentication.

### mailbox-credential

Manage mailbox credential

- **`halopsa-cli mailbox-credential create`** - Create
- **`halopsa-cli mailbox-credential delete`** - Delete
- **`halopsa-cli mailbox-credential get`** - Get
- **`halopsa-cli mailbox-credential list`** - List

### mailchimp

Manage mailchimp

- **`halopsa-cli mailchimp`** - List

### manage-engine

Manage manage engine

- **`halopsa-cli manage-engine`** - List

### manage-engine-details

Manage manage engine details

- **`halopsa-cli manage-engine-details create`** - Create
- **`halopsa-cli manage-engine-details delete`** - Delete
- **`halopsa-cli manage-engine-details get`** - Get
- **`halopsa-cli manage-engine-details list`** - List

### marketing-unsubscribe

Manage marketing unsubscribe

- **`halopsa-cli marketing-unsubscribe create`** - Create
- **`halopsa-cli marketing-unsubscribe delete`** - Delete
- **`halopsa-cli marketing-unsubscribe get`** - Get
- **`halopsa-cli marketing-unsubscribe list`** - List

### mattermost-channel-details

Manage mattermost channel details

- **`halopsa-cli mattermost-channel-details`** - List

### mattermost-details

Manage mattermost details

- **`halopsa-cli mattermost-details create`** - Create
- **`halopsa-cli mattermost-details delete`** - Delete
- **`halopsa-cli mattermost-details get`** - Get
- **`halopsa-cli mattermost-details list`** - List

### mcp

Manage mcp

- **`halopsa-cli mcp create`** - Create
- **`halopsa-cli mcp delete`** - Delete
- **`halopsa-cli mcp list`** - List

### meter-reading

Manage meter reading

- **`halopsa-cli meter-reading create`** - Create
- **`halopsa-cli meter-reading get`** - Use this to return a single instance of DeviceMeterReading.<br>
				Requires authentication.
- **`halopsa-cli meter-reading list`** - Use this to return multiple DeviceMeterReading.<br>
				Requires authentication.

### microsoft-subscription-mapping

Manage microsoft subscription mapping

- **`halopsa-cli microsoft-subscription-mapping create`** - Create
- **`halopsa-cli microsoft-subscription-mapping delete`** - Delete
- **`halopsa-cli microsoft-subscription-mapping get`** - Get
- **`halopsa-cli microsoft-subscription-mapping list`** - List

### microsoft-teams

Manage microsoft teams

- **`halopsa-cli microsoft-teams`** - List

### microsoft-teams-mapping

Manage microsoft teams mapping

- **`halopsa-cli microsoft-teams-mapping create`** - Create
- **`halopsa-cli microsoft-teams-mapping delete`** - Delete
- **`halopsa-cli microsoft-teams-mapping get`** - Get
- **`halopsa-cli microsoft-teams-mapping list`** - List

### mo

Manage mo

- **`halopsa-cli mo create`** - Create
- **`halopsa-cli mo delete`** - Delete
- **`halopsa-cli mo get`** - Get
- **`halopsa-cli mo list`** - List
- **`halopsa-cli mo list-b`** - List b
- **`halopsa-cli mo list-r`** - List r

### myobdetails

Manage myobdetails

- **`halopsa-cli myobdetails create`** - Create
- **`halopsa-cli myobdetails delete`** - Delete
- **`halopsa-cli myobdetails get`** - Get
- **`halopsa-cli myobdetails list`** - List

### ncentral-details

Manage ncentral details

- **`halopsa-cli ncentral-details create`** - Create
- **`halopsa-cli ncentral-details delete`** - Delete
- **`halopsa-cli ncentral-details get`** - Use this to return a single instance of NCentralDetails.<br>
				Requires authentication.
- **`halopsa-cli ncentral-details list`** - Use this to return multiple NCentralDetails.<br>
				Requires authentication.

### nhserverconfig

Manage nhserverconfig

- **`halopsa-cli nhserverconfig create`** - Create
- **`halopsa-cli nhserverconfig delete`** - Delete
- **`halopsa-cli nhserverconfig get`** - Use this to return a single instance of NHServerConfig.<br>
				Requires authentication.
- **`halopsa-cli nhserverconfig list`** - List

### notification

Manage notification

- **`halopsa-cli notification create`** - Create
- **`halopsa-cli notification delete`** - Delete
- **`halopsa-cli notification get`** - Use this to return a single instance of UnameNotification.<br>
				Requires authentication.
- **`halopsa-cli notification list`** - Use this to return multiple UnameNotification.<br>
				Requires authentication.

### notification-log

Manage notification log

- **`halopsa-cli notification-log`** - List

### notification-message

Manage notification message

- **`halopsa-cli notification-message create`** - Create
- **`halopsa-cli notification-message delete`** - Delete
- **`halopsa-cli notification-message get`** - Use this to return a single instance of NotificationContent.<br>
				Requires authentication.
- **`halopsa-cli notification-message list`** - List

### notifications

Manage notifications

- **`halopsa-cli notifications create`** - Create
- **`halopsa-cli notifications create-process`** - Create process
- **`halopsa-cli notifications delete`** - Delete
- **`halopsa-cli notifications get`** - Use this to return a single instance of EscMsg.<br>
				Requires authentication.
- **`halopsa-cli notifications list`** - Use this to return multiple EscMsg.<br>
				Requires authentication.

### object-mapping-profile

Manage object mapping profile

- **`halopsa-cli object-mapping-profile`** - List

### online-status

Manage online status

- **`halopsa-cli online-status create`** - Create
- **`halopsa-cli online-status list`** - List

### opportunities

Manage opportunities

- **`halopsa-cli opportunities create`** - Create
- **`halopsa-cli opportunities create-view`** - Create view
- **`halopsa-cli opportunities delete`** - Delete specific Faults.<br>
				Requires authentication.
- **`halopsa-cli opportunities get`** - Use this to return a single instance of Faults.<br>
				Requires authentication.
- **`halopsa-cli opportunities list`** - Use this to return multiple Faults.<br>
				Requires authentication.

### order-line

Manage order line

- **`halopsa-cli order-line`** - List

### organisation

Manage organisation

- **`halopsa-cli organisation create`** - Create
- **`halopsa-cli organisation delete`** - Delete
- **`halopsa-cli organisation get`** - Use this to return a single instance of Organisation.<br>
				Requires authentication.
- **`halopsa-cli organisation list`** - List

### outcome

Manage outcome

- **`halopsa-cli outcome create`** - Create
- **`halopsa-cli outcome delete`** - Delete
- **`halopsa-cli outcome get`** - Use this to return a single instance of TOutcome.<br>
				Requires authentication.
- **`halopsa-cli outcome list`** - Use this to return multiple TOutcome.<br>
				Requires authentication.

### outgoing

Manage outgoing

- **`halopsa-cli outgoing create`** - Create
- **`halopsa-cli outgoing delete`** - Delete
- **`halopsa-cli outgoing get`** - Use this to return a single instance of Outgoing.<br>
				Requires authentication.
- **`halopsa-cli outgoing list`** - Use this to return multiple Outgoing.<br>
				Requires authentication.

### outgoing-attempt

Manage outgoing attempt

- **`halopsa-cli outgoing-attempt get`** - Use this to return a single instance of OutgoingAttempt.<br>
				Requires authentication.
- **`halopsa-cli outgoing-attempt list`** - Use this to return multiple OutgoingAttempt.<br>
				Requires authentication.

### outgoingemail

Manage outgoingemail

- **`halopsa-cli outgoingemail create`** - Create
- **`halopsa-cli outgoingemail delete`** - Delete
- **`halopsa-cli outgoingemail list`** - Use this to return multiple Outgoingemail.<br>
				Requires authentication.

### pagerdutymapping

Manage pagerdutymapping

- **`halopsa-cli pagerdutymapping`** - Use this to return multiple PagerDutyMapping.<br>
				Requires authentication.

### password-field

Manage password field

- **`halopsa-cli password-field create`** - Create
- **`halopsa-cli password-field get`** - Use this to return a single instance of AuditPasswordField.<br>
				Requires authentication.
- **`halopsa-cli password-field list`** - List

### pax8-details

Manage pax8 details

- **`halopsa-cli pax8-details create`** - Create
- **`halopsa-cli pax8-details delete`** - Delete
- **`halopsa-cli pax8-details get`** - Get
- **`halopsa-cli pax8-details list`** - List

### pdf-template

Manage pdf template

- **`halopsa-cli pdf-template create`** - Create
- **`halopsa-cli pdf-template delete`** - Delete
- **`halopsa-cli pdf-template get`** - Use this to return a single instance of PdfTemplate.<br>
				Requires authentication.
- **`halopsa-cli pdf-template list`** - Use this to return multiple PdfTemplate.<br>
				Requires authentication.

### pdf-template-repository

Manage pdf template repository

- **`halopsa-cli pdf-template-repository get`** - Use this to return a single instance of PdfTemplate.<br>
				Requires authentication.
- **`halopsa-cli pdf-template-repository list`** - Use this to return multiple PdfTemplate.<br>
				Requires authentication.

### popup-note

Manage popup note

- **`halopsa-cli popup-note create`** - Create
- **`halopsa-cli popup-note list`** - Use this to return multiple AreaPopup.<br>
				Requires authentication.

### power-shell-script

Manage power shell script

- **`halopsa-cli power-shell-script create`** - Create
- **`halopsa-cli power-shell-script delete`** - Delete
- **`halopsa-cli power-shell-script get`** - Use this to return a single instance of PowerShellScript.<br>
				Requires authentication.
- **`halopsa-cli power-shell-script list`** - Use this to return multiple PowerShellScript.<br>
				Requires authentication.

### power-shell-script-criteria

Manage power shell script criteria

- **`halopsa-cli power-shell-script-criteria create`** - Create
- **`halopsa-cli power-shell-script-criteria delete`** - Delete
- **`halopsa-cli power-shell-script-criteria get`** - Use this to return a single instance of PowerShellScriptCriteria.<br>
				Requires authentication.
- **`halopsa-cli power-shell-script-criteria list`** - Use this to return multiple PowerShellScriptCriteria.<br>
				Requires authentication.

### power-shell-script-processing

Manage power shell script processing

- **`halopsa-cli power-shell-script-processing create`** - Create
- **`halopsa-cli power-shell-script-processing delete`** - Delete
- **`halopsa-cli power-shell-script-processing get`** - Use this to return a single instance of PowerShellScriptProcessing.<br>
				Requires authentication.
- **`halopsa-cli power-shell-script-processing list`** - Use this to return multiple PowerShellScriptProcessing.<br>
				Requires authentication.

### priority

Manage priority

- **`halopsa-cli priority create`** - Create
- **`halopsa-cli priority delete`** - Delete
- **`halopsa-cli priority get`** - Use this to return a single instance of Policy.<br>
				Requires authentication.
- **`halopsa-cli priority list`** - Use this to return multiple Policy.<br>
				Requires authentication.

### product

Manage product

- **`halopsa-cli product create`** - Create
- **`halopsa-cli product delete`** - Delete
- **`halopsa-cli product get`** - Use this to return a single instance of ReleaseProduct.<br>
				Requires authentication.
- **`halopsa-cli product list`** - Use this to return multiple ReleaseProduct.<br>
				Requires authentication.

### product-branch

Manage product branch

- **`halopsa-cli product-branch`** - Use this to return multiple ReleaseBranch.<br>
				Requires authentication.

### product-component

Manage product component

- **`halopsa-cli product-component create`** - Create
- **`halopsa-cli product-component delete`** - Delete
- **`halopsa-cli product-component get`** - Use this to return a single instance of ReleaseComponent.<br>
				Requires authentication.
- **`halopsa-cli product-component list`** - Use this to return multiple ReleaseComponent.<br>
				Requires authentication.

### project-setup-lines

Manage project setup lines

- **`halopsa-cli project-setup-lines`** - Create

### projects

Manage projects

- **`halopsa-cli projects create`** - Create
- **`halopsa-cli projects create-view`** - Create view
- **`halopsa-cli projects delete`** - Delete specific Faults.<br>
				Requires authentication.
- **`halopsa-cli projects get`** - Use this to return a single instance of Faults.<br>
				Requires authentication.
- **`halopsa-cli projects list`** - Use this to return multiple Faults.<br>
				Requires authentication.

### prtgdetails

Manage prtgdetails

- **`halopsa-cli prtgdetails create`** - Create
- **`halopsa-cli prtgdetails delete`** - Delete
- **`halopsa-cli prtgdetails get`** - Get
- **`halopsa-cli prtgdetails list`** - List

### publish-profiles

Manage publish profiles

- **`halopsa-cli publish-profiles create`** - Create
- **`halopsa-cli publish-profiles delete`** - Delete
- **`halopsa-cli publish-profiles get`** - Get
- **`halopsa-cli publish-profiles list`** - List

### purchase-order

Manage purchase order

- **`halopsa-cli purchase-order create`** - Create
- **`halopsa-cli purchase-order create-purchaseorder`** - Create purchaseorder
- **`halopsa-cli purchase-order create-purchaseorder-2`** - Create purchaseorder 2
- **`halopsa-cli purchase-order delete`** - Delete
- **`halopsa-cli purchase-order get`** - Use this to return a single instance of SupplierOrderHeader.<br>
				Requires authentication.
- **`halopsa-cli purchase-order list`** - Use this to return multiple SupplierOrderHeader.<br>
				Requires authentication.

### qualification

Manage qualification

- **`halopsa-cli qualification create`** - Create
- **`halopsa-cli qualification delete`** - Delete
- **`halopsa-cli qualification get`** - Use this to return a single instance of Qualification.<br>
				Requires authentication.
- **`halopsa-cli qualification list`** - Use this to return multiple Qualification.<br>
				Requires authentication.

### quick-books-details

Manage quick books details

- **`halopsa-cli quick-books-details create`** - Create
- **`halopsa-cli quick-books-details delete`** - Delete
- **`halopsa-cli quick-books-details get`** - Use this to return a single instance of QuickBooksDetails.<br>
				Requires authentication.
- **`halopsa-cli quick-books-details list`** - Use this to return multiple QuickBooksDetails.<br>
				Requires authentication.

### quotation

Manage quotation

- **`halopsa-cli quotation create`** - Create
- **`halopsa-cli quotation create-approval`** - Create approval
- **`halopsa-cli quotation create-lines`** - Create lines
- **`halopsa-cli quotation create-view`** - Create view
- **`halopsa-cli quotation delete`** - Delete
- **`halopsa-cli quotation get`** - Use this to return a single instance of QuotationHeader.<br>
				Requires authentication.
- **`halopsa-cli quotation list`** - Use this to return multiple QuotationHeader.<br>
				Requires authentication.

### raynet

Manage raynet

- **`halopsa-cli raynet`** - List

### raynet-details

Manage raynet details

- **`halopsa-cli raynet-details create`** - Create
- **`halopsa-cli raynet-details delete`** - Delete
- **`halopsa-cli raynet-details get`** - Get
- **`halopsa-cli raynet-details list`** - List

### recurring-invoice

Manage recurring invoice

- **`halopsa-cli recurring-invoice create`** - Create
- **`halopsa-cli recurring-invoice create-recurringinvoice`** - Create recurringinvoice
- **`halopsa-cli recurring-invoice create-recurringinvoice-2`** - Create recurringinvoice 2
- **`halopsa-cli recurring-invoice create-recurringinvoice-3`** - Create recurringinvoice 3
- **`halopsa-cli recurring-invoice delete`** - Delete specific InvoiceHeader.<br>
				Requires authentication.
- **`halopsa-cli recurring-invoice get`** - Use this to return a single instance of InvoiceHeader.<br>
				Requires authentication.
- **`halopsa-cli recurring-invoice list`** - Use this to return multiple InvoiceHeader.<br>
				Requires authentication.

### recurring-item

Manage recurring item

- **`halopsa-cli recurring-item`** - Use this to return multiple AreaItem.<br>
				Requires authentication.

### release

Manage release

- **`halopsa-cli release create`** - Create
- **`halopsa-cli release delete`** - Delete
- **`halopsa-cli release get`** - Use this to return a single instance of Release.<br>
				Requires authentication.
- **`halopsa-cli release list`** - .<br>
				Requires authentication.

### release-note-group

Manage release note group

- **`halopsa-cli release-note-group create`** - Create
- **`halopsa-cli release-note-group delete`** - Delete
- **`halopsa-cli release-note-group get`** - Use this to return a single instance of ReleaseNoteGroup.<br>
				Requires authentication.
- **`halopsa-cli release-note-group list`** - List

### release-pipeline

Manage release pipeline

- **`halopsa-cli release-pipeline create`** - Create
- **`halopsa-cli release-pipeline delete`** - Delete
- **`halopsa-cli release-pipeline get`** - Get
- **`halopsa-cli release-pipeline list`** - List

### release-type

Manage release type

- **`halopsa-cli release-type create`** - Create
- **`halopsa-cli release-type delete`** - Delete
- **`halopsa-cli release-type get`** - Use this to return a single instance of ReleaseType.<br>
				Requires authentication.
- **`halopsa-cli release-type list`** - List

### remote-session

Manage remote session

- **`halopsa-cli remote-session create`** - Create
- **`halopsa-cli remote-session delete`** - Delete
- **`halopsa-cli remote-session get`** - Use this to return a single instance of RemoteSessionData.<br>
				Requires authentication.
- **`halopsa-cli remote-session list`** - Use this to return multiple RemoteSessionData.<br>
				Requires authentication.

### remote-session-teams

Manage remote session teams

- **`halopsa-cli remote-session-teams`** - Use this to return multiple RemoteSessionTeams.<br>
				Requires authentication.

### report

Manage report

- **`halopsa-cli report create`** - Create
- **`halopsa-cli report create-bookmark`** - Create bookmark
- **`halopsa-cli report create-createpdf`** - Create createpdf
- **`halopsa-cli report create-print`** - Create print
- **`halopsa-cli report delete`** - Delete
- **`halopsa-cli report get`** - Use this to return a single instance of AnalyzerProfile.<br>
				Requires authentication.
- **`halopsa-cli report list`** - Use this to return multiple AnalyzerProfile.<br>
				Requires authentication.

### report-data

Manage report data

- **`halopsa-cli report-data <publishedid>`** - Get

### report-repository

Manage report repository

- **`halopsa-cli report-repository get`** - Use this to return a single instance of AnalyzerProfile.<br>
				Requires authentication.
- **`halopsa-cli report-repository list`** - Use this to return multiple AnalyzerProfile.<br>
				Requires authentication.
- **`halopsa-cli report-repository list-reportrepository`** - Use this to return multiple Lookup.<br>
				Requires authentication.

### resource-type

Manage resource type

- **`halopsa-cli resource-type get`** - Get
- **`halopsa-cli resource-type list`** - List

### roadmap

Manage roadmap

- **`halopsa-cli roadmap`** - .<br>
				Requires authentication.

### roles

Manage roles

- **`halopsa-cli roles create`** - Create
- **`halopsa-cli roles delete`** - Delete
- **`halopsa-cli roles get`** - Use this to return a single instance of NHD_Roles.<br>
				Requires authentication.
- **`halopsa-cli roles list`** - Use this to return multiple NHD_Roles.<br>
				Requires authentication.

### sage-business-cloud-details

Manage sage business cloud details

- **`halopsa-cli sage-business-cloud-details create`** - Create
- **`halopsa-cli sage-business-cloud-details delete`** - Delete
- **`halopsa-cli sage-business-cloud-details get`** - Use this to return a single instance of SageBusinessCloudDetails.<br>
				Requires authentication.
- **`halopsa-cli sage-business-cloud-details list`** - Use this to return multiple SageBusinessCloudDetails.<br>
				Requires authentication.

### sail-point-details

Manage sail point details

- **`halopsa-cli sail-point-details create`** - Create
- **`halopsa-cli sail-point-details delete`** - Delete
- **`halopsa-cli sail-point-details get`** - Get
- **`halopsa-cli sail-point-details list`** - List

### sail-point-role-mapping

Manage sail point role mapping

- **`halopsa-cli sail-point-role-mapping`** - List

### sail-point-user-mapping

Manage sail point user mapping

- **`halopsa-cli sail-point-user-mapping`** - List

### sales-mailbox

Manage sales mailbox

- **`halopsa-cli sales-mailbox create`** - Create
- **`halopsa-cli sales-mailbox delete`** - Delete
- **`halopsa-cli sales-mailbox get`** - Use this to return a single instance of SalesMailbox.<br>
				Requires authentication.
- **`halopsa-cli sales-mailbox list`** - List

### sales-mailbox-detail

Manage sales mailbox detail

- **`halopsa-cli sales-mailbox-detail create`** - Create
- **`halopsa-cli sales-mailbox-detail list`** - List

### sales-order

Manage sales order

- **`halopsa-cli sales-order create`** - Create
- **`halopsa-cli sales-order create-salesorder`** - Create salesorder
- **`halopsa-cli sales-order delete`** - Delete
- **`halopsa-cli sales-order get`** - Use this to return a single instance of OrderHead.<br>
				Requires authentication.
- **`halopsa-cli sales-order list`** - Use this to return multiple OrderHead.<br>
				Requires authentication.

### saved-forecast

Manage saved forecast

- **`halopsa-cli saved-forecast create`** - Create
- **`halopsa-cli saved-forecast delete`** - Delete
- **`halopsa-cli saved-forecast get`** - Get
- **`halopsa-cli saved-forecast list`** - List

### schedule

Manage schedule

- **`halopsa-cli schedule create`** - Create
- **`halopsa-cli schedule get`** - Use this to return a single instance of Schedule.<br>
				Requires authentication.
- **`halopsa-cli schedule list`** - Use this to return multiple Schedule.<br>
				Requires authentication.

### schedule-occurrence

Manage schedule occurrence

- **`halopsa-cli schedule-occurrence create`** - Create
- **`halopsa-cli schedule-occurrence get`** - Get
- **`halopsa-cli schedule-occurrence list`** - List

### screen-layout

Manage screen layout

- **`halopsa-cli screen-layout create`** - Create
- **`halopsa-cli screen-layout delete`** - Delete
- **`halopsa-cli screen-layout get`** - Use this to return a single instance of ScreenLayout.<br>
				Requires authentication.
- **`halopsa-cli screen-layout list`** - Use this to return multiple ScreenLayout.<br>
				Requires authentication.

### secure-secret-link

Manage secure secret link

- **`halopsa-cli secure-secret-link create`** - Create
- **`halopsa-cli secure-secret-link delete`** - Delete
- **`halopsa-cli secure-secret-link get`** - Get
- **`halopsa-cli secure-secret-link list`** - List
- **`halopsa-cli secure-secret-link list-securesecretlink`** - List securesecretlink

### security-check

Manage security check

- **`halopsa-cli security-check list`** - List
- **`halopsa-cli security-check list-securitycheck`** - List securitycheck

### security-question

Manage security question

- **`halopsa-cli security-question create`** - Create
- **`halopsa-cli security-question delete`** - Delete
- **`halopsa-cli security-question get`** - Use this to return a single instance of SecurityQuestion.<br>
				Requires authentication.
- **`halopsa-cli security-question list`** - List

### security-question-validate

Manage security question validate

- **`halopsa-cli security-question-validate create`** - Create
- **`halopsa-cli security-question-validate list`** - List

### sentinel-one

Manage sentinel one

- **`halopsa-cli sentinel-one`** - List

### sentinel-one-details

Manage sentinel one details

- **`halopsa-cli sentinel-one-details create`** - Create
- **`halopsa-cli sentinel-one-details delete`** - Delete
- **`halopsa-cli sentinel-one-details get`** - Get
- **`halopsa-cli sentinel-one-details list`** - List

### service

Manage service

- **`halopsa-cli service create`** - Create
- **`halopsa-cli service create-unsubscribe`** - Create unsubscribe
- **`halopsa-cli service delete`** - Delete
- **`halopsa-cli service get`** - Use this to return a single instance of ServSite.<br>
				Requires authentication.
- **`halopsa-cli service list`** - Use this to return multiple ServSite.<br>
				Requires authentication.

### service-availability

Manage service availability

- **`halopsa-cli service-availability create`** - Create
- **`halopsa-cli service-availability delete`** - Delete
- **`halopsa-cli service-availability get`** - Get
- **`halopsa-cli service-availability list`** - List

### service-category

Manage service category

- **`halopsa-cli service-category create`** - Create
- **`halopsa-cli service-category delete`** - Delete
- **`halopsa-cli service-category get`** - Use this to return a single instance of ServiceCategory.<br>
				Requires authentication.
- **`halopsa-cli service-category list`** - Use this to return multiple ServiceCategory.<br>
				Requires authentication.

### service-request-details

Manage service request details

- **`halopsa-cli service-request-details get`** - Use this to return a single instance of ServiceRequestDetails.<br>
				Requires authentication.
- **`halopsa-cli service-request-details list`** - Use this to return multiple ServiceRequestDetails.<br>
				Requires authentication.

### service-restriction

Manage service restriction

- **`halopsa-cli service-restriction`** - Use this to return multiple ServiceRestriction.<br>
				Requires authentication.

### service-status

Manage service status

- **`halopsa-cli service-status create`** - Create
- **`halopsa-cli service-status create-servicestatus`** - Create servicestatus
- **`halopsa-cli service-status delete`** - Delete
- **`halopsa-cli service-status get`** - Use this to return a single instance of ServStatus.<br>
				Requires authentication.
- **`halopsa-cli service-status get-servicestatus`** - Get servicestatus
- **`halopsa-cli service-status list`** - Use this to return multiple ServStatus.<br>
				Requires authentication.

### setup-tab

Manage setup tab

- **`halopsa-cli setup-tab create`** - Create
- **`halopsa-cli setup-tab get`** - Use this to return a single instance of SetupTab.<br>
				Requires authentication.
- **`halopsa-cli setup-tab list`** - List

### setup-tab-group

Manage setup tab group

- **`halopsa-cli setup-tab-group get`** - Use this to return a single instance of SetupTabGroup.<br>
				Requires authentication.
- **`halopsa-cli setup-tab-group list`** - List

### share-point

Manage share point

- **`halopsa-cli share-point`** - List

### shopify-details

Manage shopify details

- **`halopsa-cli shopify-details create`** - Create
- **`halopsa-cli shopify-details delete`** - Delete
- **`halopsa-cli shopify-details get`** - Get
- **`halopsa-cli shopify-details list`** - List

### single-sign-on-application

Manage single sign on application

- **`halopsa-cli single-sign-on-application create`** - Create
- **`halopsa-cli single-sign-on-application delete`** - Delete
- **`halopsa-cli single-sign-on-application get`** - Get
- **`halopsa-cli single-sign-on-application list`** - List

### single-sign-on-attempt

Manage single sign on attempt

- **`halopsa-cli single-sign-on-attempt delete`** - Delete
- **`halopsa-cli single-sign-on-attempt get`** - Get
- **`halopsa-cli single-sign-on-attempt list`** - List

### site

Manage site

- **`halopsa-cli site create`** - Create
- **`halopsa-cli site delete`** - Delete
- **`halopsa-cli site get`** - Use this to return a single instance of Site.<br>
				Requires authentication.
- **`halopsa-cli site list`** - Use this to return multiple Site.<br>
				Requires authentication.
- **`halopsa-cli site list-stockbins`** - List stockbins

### sla

Manage sla

- **`halopsa-cli sla create`** - Create
- **`halopsa-cli sla delete`** - Delete
- **`halopsa-cli sla get`** - Use this to return a single instance of SlaHead.<br>
				Requires authentication.
- **`halopsa-cli sla list`** - Use this to return multiple SlaHead.<br>
				Requires authentication.

### slack

Manage slack

- **`halopsa-cli slack create`** - Create
- **`halopsa-cli slack create-event`** - Create event
- **`halopsa-cli slack create-interactivity`** - Create interactivity
- **`halopsa-cli slack create-manifest`** - Create manifest

### slack-chat-app

Manage slack chat app

- **`halopsa-cli slack-chat-app create`** - Create
- **`halopsa-cli slack-chat-app delete`** - Delete
- **`halopsa-cli slack-chat-app get`** - Get
- **`halopsa-cli slack-chat-app list`** - List

### slack-details

Manage slack details

- **`halopsa-cli slack-details create`** - Create
- **`halopsa-cli slack-details create-slackdetails`** - Create slackdetails
- **`halopsa-cli slack-details delete`** - Delete
- **`halopsa-cli slack-details get`** - Use this to return a single instance of SlackDetails.<br>
				Requires authentication.
- **`halopsa-cli slack-details list`** - Use this to return multiple SlackDetails.<br>
				Requires authentication.

### snipe-itdetails

Manage snipe itdetails

- **`halopsa-cli snipe-itdetails create`** - Create
- **`halopsa-cli snipe-itdetails delete`** - Delete
- **`halopsa-cli snipe-itdetails get`** - Get
- **`halopsa-cli snipe-itdetails list`** - List

### snow-details

Manage snow details

- **`halopsa-cli snow-details create`** - Create
- **`halopsa-cli snow-details delete`** - Delete
- **`halopsa-cli snow-details get`** - Use this to return a single instance of SnowDetails.<br>
				Requires authentication.
- **`halopsa-cli snow-details list`** - Use this to return multiple SnowDetails.<br>
				Requires authentication.

### software-licence

Manage software licence

- **`halopsa-cli software-licence create`** - Create
- **`halopsa-cli software-licence delete`** - Delete
- **`halopsa-cli software-licence get`** - Use this to return a single instance of Licence.<br>
				Requires authentication.
- **`halopsa-cli software-licence list`** - Use this to return multiple Licence.<br>
				Requires authentication.

### software-licence-role

Manage software licence role

- **`halopsa-cli software-licence-role`** - Use this to return multiple LicenceRole.<br>
				Requires authentication.

### sophos

Manage sophos

- **`halopsa-cli sophos`** - List

### sophos-details

Manage sophos details

- **`halopsa-cli sophos-details create`** - Create
- **`halopsa-cli sophos-details delete`** - Delete
- **`halopsa-cli sophos-details get`** - Get
- **`halopsa-cli sophos-details list`** - List

### sqlimport

Manage sqlimport

- **`halopsa-cli sqlimport create`** - Create
- **`halopsa-cli sqlimport delete`** - Delete
- **`halopsa-cli sqlimport get`** - Use this to return a single instance of SQLImport.<br>
				Requires authentication.
- **`halopsa-cli sqlimport list`** - Use this to return multiple SQLImport.<br>
				Requires authentication.

### status

Manage status

- **`halopsa-cli status create`** - Create
- **`halopsa-cli status delete`** - Delete
- **`halopsa-cli status get`** - Use this to return a single instance of TStatus.<br>
				Requires authentication.
- **`halopsa-cli status list`** - Use this to return multiple TStatus.<br>
				Requires authentication.

### stock-bin

Manage stock bin

- **`halopsa-cli stock-bin create`** - Create
- **`halopsa-cli stock-bin delete`** - Delete
- **`halopsa-cli stock-bin get`** - Get
- **`halopsa-cli stock-bin list`** - List

### stock-trace

Manage stock trace

- **`halopsa-cli stock-trace get`** - Get
- **`halopsa-cli stock-trace list`** - List

### stream-one-ion-details

Manage stream one ion details

- **`halopsa-cli stream-one-ion-details create`** - Create
- **`halopsa-cli stream-one-ion-details delete`** - Delete
- **`halopsa-cli stream-one-ion-details get`** - Get
- **`halopsa-cli stream-one-ion-details list`** - List

### style-profile

Manage style profile

- **`halopsa-cli style-profile create`** - Create
- **`halopsa-cli style-profile delete`** - Delete
- **`halopsa-cli style-profile get`** - Get
- **`halopsa-cli style-profile list`** - List

### supplier

Manage supplier

- **`halopsa-cli supplier create`** - Create
- **`halopsa-cli supplier delete`** - Delete
- **`halopsa-cli supplier get`** - Use this to return a single instance of Company.<br>
				Requires authentication.
- **`halopsa-cli supplier list`** - Use this to return multiple Company.<br>
				Requires authentication.

### supplier-contract

Manage supplier contract

- **`halopsa-cli supplier-contract create`** - Create
- **`halopsa-cli supplier-contract create-suppliercontract`** - Create suppliercontract
- **`halopsa-cli supplier-contract delete`** - Delete
- **`halopsa-cli supplier-contract get`** - Use this to return a single instance of Contract.<br>
				Requires authentication.
- **`halopsa-cli supplier-contract list`** - Use this to return multiple Contract.<br>
				Requires authentication.

### synnex-details

Manage synnex details

- **`halopsa-cli synnex-details create`** - Create
- **`halopsa-cli synnex-details delete`** - Delete
- **`halopsa-cli synnex-details get`** - Use this to return a single instance of IngramMicroDetails.<br>
				Requires authentication.
- **`halopsa-cli synnex-details list`** - List

### tabs

Manage tabs

- **`halopsa-cli tabs create`** - Create
- **`halopsa-cli tabs delete`** - Delete
- **`halopsa-cli tabs get`** - Use this to return a single instance of Tabname.<br>
				Requires authentication.
- **`halopsa-cli tabs list`** - Use this to return multiple Tabname.<br>
				Requires authentication.

### tags

Manage tags

- **`halopsa-cli tags create`** - Create
- **`halopsa-cli tags delete`** - Delete
- **`halopsa-cli tags get`** - Use this to return a single instance of Tag.<br>
				Requires authentication.
- **`halopsa-cli tags list`** - List

### take-control

Manage take control

- **`halopsa-cli take-control`** - List

### tanium-details

Manage tanium details

- **`halopsa-cli tanium-details create`** - Create
- **`halopsa-cli tanium-details delete`** - Delete
- **`halopsa-cli tanium-details get`** - Get
- **`halopsa-cli tanium-details list`** - List

### task-monitor-event

Manage task monitor event

- **`halopsa-cli task-monitor-event`** - List

### task-schedule

Manage task schedule

- **`halopsa-cli task-schedule create`** - Create
- **`halopsa-cli task-schedule list`** - List

### task-trace

Manage task trace

- **`halopsa-cli task-trace get`** - Get
- **`halopsa-cli task-trace list`** - List

### tax

Manage tax

- **`halopsa-cli tax create`** - Create
- **`halopsa-cli tax delete`** - Delete
- **`halopsa-cli tax get`** - Use this to return a single instance of Tax.<br>
				Requires authentication.
- **`halopsa-cli tax list`** - Use this to return multiple Tax.<br>
				Requires authentication.

### tax-rule

Manage tax rule

- **`halopsa-cli tax-rule create`** - Create
- **`halopsa-cli tax-rule delete`** - Delete
- **`halopsa-cli tax-rule get`** - Get
- **`halopsa-cli tax-rule list`** - List

### team

Manage team

- **`halopsa-cli team create`** - Create
- **`halopsa-cli team delete`** - Delete
- **`halopsa-cli team get`** - Use this to return a single instance of SectionDetail.<br>
				Requires authentication.
- **`halopsa-cli team list`** - Use this to return multiple SectionDetail.<br>
				Requires authentication.
- **`halopsa-cli team list-tree`** - List tree

### team-image

Manage team image

- **`halopsa-cli team-image <id>`** - Get

### tech-data-reseller-details

Manage tech data reseller details

- **`halopsa-cli tech-data-reseller-details create`** - Create
- **`halopsa-cli tech-data-reseller-details delete`** - Delete
- **`halopsa-cli tech-data-reseller-details get`** - Get
- **`halopsa-cli tech-data-reseller-details list`** - List

### template

Manage template

- **`halopsa-cli template create`** - Create
- **`halopsa-cli template delete`** - Delete
- **`halopsa-cli template get`** - Use this to return a single instance of StdRequest.<br>
				Requires authentication.
- **`halopsa-cli template list`** - Use this to return multiple StdRequest.<br>
				Requires authentication.

### tenable

Manage tenable

- **`halopsa-cli tenable create`** - Create
- **`halopsa-cli tenable create-export`** - Create export
- **`halopsa-cli tenable list`** - List
- **`halopsa-cli tenable list-status`** - List status

### tenable-details

Manage tenable details

- **`halopsa-cli tenable-details create`** - Create
- **`halopsa-cli tenable-details delete`** - Delete
- **`halopsa-cli tenable-details get`** - Get
- **`halopsa-cli tenable-details list`** - List

### tenant

Manage tenant

- **`halopsa-cli tenant create`** - Create
- **`halopsa-cli tenant list`** - List

### test-error

Manage test error

- **`halopsa-cli test-error`** - List

### test1

Manage test1

- **`halopsa-cli test1`** - List

### test3

Manage test3

- **`halopsa-cli test3`** - List

### test4

Manage test4

- **`halopsa-cli test4`** - List

### ticket-approval

Manage ticket approval

- **`halopsa-cli ticket-approval create`** - Create
- **`halopsa-cli ticket-approval delete`** - Delete
- **`halopsa-cli ticket-approval get`** - Use this to return a single instance of FaultApproval.<br>
				Requires authentication.
- **`halopsa-cli ticket-approval list`** - Use this to return multiple FaultApproval.<br>
				Requires authentication.

### ticket-area

Manage ticket area

- **`halopsa-cli ticket-area create`** - Create
- **`halopsa-cli ticket-area delete`** - Delete
- **`halopsa-cli ticket-area get`** - Use this to return a single instance of TicketArea.<br>
				Requires authentication.
- **`halopsa-cli ticket-area list`** - List

### ticket-rules

Manage ticket rules

- **`halopsa-cli ticket-rules create`** - Create
- **`halopsa-cli ticket-rules delete`** - Delete
- **`halopsa-cli ticket-rules get`** - Use this to return a single instance of Autoassign.<br>
				Requires authentication.
- **`halopsa-cli ticket-rules list`** - Use this to return multiple Autoassign.<br>
				Requires authentication.

### ticket-type

Manage ticket type

- **`halopsa-cli ticket-type create`** - Create
- **`halopsa-cli ticket-type delete`** - Delete
- **`halopsa-cli ticket-type get`** - Use this to return a single instance of RequestType.<br>
				Requires authentication.
- **`halopsa-cli ticket-type list`** - Use this to return multiple RequestType.<br>
				Requires authentication.

### ticket-type-field

Manage ticket type field

- **`halopsa-cli ticket-type-field`** - Use this to return multiple RequestTypeField.<br>
				Requires authentication.

### ticket-type-group

Manage ticket type group

- **`halopsa-cli ticket-type-group create`** - Create
- **`halopsa-cli ticket-type-group delete`** - Delete
- **`halopsa-cli ticket-type-group get`** - Use this to return a single instance of RequestTypeGroup.<br>
				Requires authentication.
- **`halopsa-cli ticket-type-group list`** - List

### tickets

Manage tickets

- **`halopsa-cli tickets create`** - Create
- **`halopsa-cli tickets create-object`** - Create object
- **`halopsa-cli tickets create-processchildren`** - Create processchildren
- **`halopsa-cli tickets create-setbillableproject`** - Create setbillableproject
- **`halopsa-cli tickets create-view`** - Create view
- **`halopsa-cli tickets create-vote`** - Create vote
- **`halopsa-cli tickets delete`** - Delete specific Faults.<br>
				Requires authentication.
- **`halopsa-cli tickets get`** - Use this to return a single instance of Faults.<br>
				Requires authentication.
- **`halopsa-cli tickets list`** - Use this to return multiple Faults.<br>
				Requires authentication.
- **`halopsa-cli tickets list-salesmailbox`** - List salesmailbox
- **`halopsa-cli tickets list-zapier`** - List zapier

### timesheet

Manage timesheet

- **`halopsa-cli timesheet create`** - Create
- **`halopsa-cli timesheet get`** - Use this to return a single instance of Timesheet.<br>
				Requires authentication.
- **`halopsa-cli timesheet list`** - List
- **`halopsa-cli timesheet list-forecasting`** - List forecasting
- **`halopsa-cli timesheet list-mine`** - List mine

### timesheet-event

Manage timesheet event

- **`halopsa-cli timesheet-event create`** - Create
- **`halopsa-cli timesheet-event delete`** - Delete
- **`halopsa-cli timesheet-event get`** - Use this to return a single instance of TimesheetEvent.<br>
				Requires authentication.
- **`halopsa-cli timesheet-event list`** - Use this to return multiple TimesheetEvent.<br>
				Requires authentication.
- **`halopsa-cli timesheet-event list-timesheetevent`** - List timesheetevent

### timeslot

Manage timeslot

- **`halopsa-cli timeslot`** - Use this to return multiple Timeslot.<br>
				Requires authentication.

### to-do

Manage to do

- **`halopsa-cli to-do create`** - Create
- **`halopsa-cli to-do list`** - Use this to return multiple FaultToDo.<br>
				Requires authentication.

### to-do-group

Manage to do group

- **`halopsa-cli to-do-group create`** - Create
- **`halopsa-cli to-do-group delete`** - Delete
- **`halopsa-cli to-do-group get`** - Get
- **`halopsa-cli to-do-group list`** - List

### top-level

Manage top level

- **`halopsa-cli top-level create`** - Create
- **`halopsa-cli top-level delete`** - Delete
- **`halopsa-cli top-level get`** - Use this to return a single instance of Tree.<br>
				Requires authentication.
- **`halopsa-cli top-level list`** - Use this to return multiple Tree.<br>
				Requires authentication.

### transcription-store

Manage transcription store

- **`halopsa-cli transcription-store create`** - Create
- **`halopsa-cli transcription-store delete`** - Delete
- **`halopsa-cli transcription-store get`** - Get
- **`halopsa-cli transcription-store list`** - List

### translation

Manage translation

- **`halopsa-cli translation create`** - Create
- **`halopsa-cli translation list`** - List

### twilio

Manage twilio

- **`halopsa-cli twilio create`** - Create
- **`halopsa-cli twilio create-twiml`** - Create twiml

### twilio-details

Manage twilio details

- **`halopsa-cli twilio-details`** - List

### twilio-whats-app-details

Manage twilio whats app details

- **`halopsa-cli twilio-whats-app-details create`** - Create
- **`halopsa-cli twilio-whats-app-details delete`** - Delete
- **`halopsa-cli twilio-whats-app-details get`** - Get
- **`halopsa-cli twilio-whats-app-details list`** - List

### twitter-details

Manage twitter details

- **`halopsa-cli twitter-details create`** - Create
- **`halopsa-cli twitter-details delete`** - Delete
- **`halopsa-cli twitter-details get`** - Use this to return a single instance of TwitterDetails.<br>
				Requires authentication.
- **`halopsa-cli twitter-details list`** - Use this to return multiple TwitterDetails.<br>
				Requires authentication.

### unsub-service-emails

Manage unsub service emails

- **`halopsa-cli unsub-service-emails create`** - Create
- **`halopsa-cli unsub-service-emails delete`** - Delete
- **`halopsa-cli unsub-service-emails get`** - Use this to return a single instance of UnsubEmailServiceUsers.<br>
				Requires authentication.
- **`halopsa-cli unsub-service-emails list`** - List

### user-change

Manage user change

- **`halopsa-cli user-change`** - Use this to return multiple UserChange.<br>
				Requires authentication.

### user-roles

Manage user roles

- **`halopsa-cli user-roles create`** - Create
- **`halopsa-cli user-roles delete`** - Delete
- **`halopsa-cli user-roles get`** - Use this to return a single instance of UserRoles.<br>
				Requires authentication.
- **`halopsa-cli user-roles list`** - List

### users

Manage users

- **`halopsa-cli users create`** - Create
- **`halopsa-cli users create-prefs`** - Create prefs
- **`halopsa-cli users delete`** - Delete
- **`halopsa-cli users get`** - Use this to return a single instance of Users.<br>
				Requires authentication.
- **`halopsa-cli users list`** - Use this to return multiple Users.<br>
				Requires authentication.
- **`halopsa-cli users list-me`** - List me

### version-info

Manage version info

- **`halopsa-cli version-info get`** - Use this to return a single instance of Release.<br>
				Requires authentication.
- **`halopsa-cli version-info get-versioninfo`** - Get versioninfo
- **`halopsa-cli version-info list`** - .<br>
				Requires authentication.
- **`halopsa-cli version-info list-versioninfo`** - List versioninfo
- **`halopsa-cli version-info list-versioninfo-2`** - .<br>
				Requires authentication.
- **`halopsa-cli version-info list-versioninfo-3`** - .<br>
				Requires authentication.

### view-columns

Manage view columns

- **`halopsa-cli view-columns create`** - Create
- **`halopsa-cli view-columns delete`** - Delete
- **`halopsa-cli view-columns get`** - Use this to return a single instance of ViewColumns.<br>
				Requires authentication.
- **`halopsa-cli view-columns list`** - Use this to return multiple ViewColumns.<br>
				Requires authentication.

### view-filter

Manage view filter

- **`halopsa-cli view-filter create`** - Create
- **`halopsa-cli view-filter delete`** - Delete
- **`halopsa-cli view-filter get`** - Use this to return a single instance of ViewFilter.<br>
				Requires authentication.
- **`halopsa-cli view-filter list`** - Use this to return multiple ViewFilter.<br>
				Requires authentication.

### view-list-group

Manage view list group

- **`halopsa-cli view-list-group create`** - Create
- **`halopsa-cli view-list-group delete`** - Delete
- **`halopsa-cli view-list-group get`** - Use this to return a single instance of ViewListGroup.<br>
				Requires authentication.
- **`halopsa-cli view-list-group list`** - Use this to return multiple ViewListGroup.<br>
				Requires authentication.

### view-lists

Manage view lists

- **`halopsa-cli view-lists create`** - Create
- **`halopsa-cli view-lists delete`** - Delete
- **`halopsa-cli view-lists get`** - Use this to return a single instance of ViewLists.<br>
				Requires authentication.
- **`halopsa-cli view-lists list`** - Use this to return multiple ViewLists.<br>
				Requires authentication.

### virima

Manage virima

- **`halopsa-cli virima`** - List

### virima-details

Manage virima details

- **`halopsa-cli virima-details create`** - Create
- **`halopsa-cli virima-details delete`** - Delete
- **`halopsa-cli virima-details get`** - Get
- **`halopsa-cli virima-details list`** - List

### virtual-agent

Manage virtual agent

- **`halopsa-cli virtual-agent create`** - Create
- **`halopsa-cli virtual-agent delete`** - Delete
- **`halopsa-cli virtual-agent get`** - Get
- **`halopsa-cli virtual-agent list`** - List

### vmworkspace-details

Manage vmworkspace details

- **`halopsa-cli vmworkspace-details create`** - Create
- **`halopsa-cli vmworkspace-details delete`** - Delete
- **`halopsa-cli vmworkspace-details get`** - Get
- **`halopsa-cli vmworkspace-details list`** - List

### vorboss

Manage vorboss

- **`halopsa-cli vorboss`** - List

### webhook

Manage webhook

- **`halopsa-cli webhook create`** - Create
- **`halopsa-cli webhook delete`** - Delete
- **`halopsa-cli webhook get`** - Use this to return a single instance of Webhook.<br>
				Requires authentication.
- **`halopsa-cli webhook list`** - Use this to return multiple Webhook.<br>
				Requires authentication.

### webhook-event

Manage webhook event

- **`halopsa-cli webhook-event create`** - Create
- **`halopsa-cli webhook-event get`** - Use this to return a single instance of WebhookEvent.<br>
				Requires authentication.
- **`halopsa-cli webhook-event list`** - Use this to return multiple WebhookEvent.<br>
				Requires authentication.

### webhook-repository

Manage webhook repository

- **`halopsa-cli webhook-repository get`** - Use this to return a single instance of Webhook.<br>
				Requires authentication.
- **`halopsa-cli webhook-repository list`** - Use this to return multiple Webhook.<br>
				Requires authentication.

### whats-app

Manage whats app

- **`halopsa-cli whats-app list`** - List
- **`halopsa-cli whats-app list-whatsapp`** - List whatsapp

### wordpress-details

Manage wordpress details

- **`halopsa-cli wordpress-details create`** - Create
- **`halopsa-cli wordpress-details delete`** - Delete
- **`halopsa-cli wordpress-details get`** - Get
- **`halopsa-cli wordpress-details list`** - List

### wordpress-org-details

Manage wordpress org details

- **`halopsa-cli wordpress-org-details create`** - Create
- **`halopsa-cli wordpress-org-details delete`** - Delete
- **`halopsa-cli wordpress-org-details get`** - Get
- **`halopsa-cli wordpress-org-details list`** - List

### workday

Manage workday

- **`halopsa-cli workday create`** - Create
- **`halopsa-cli workday delete`** - Delete
- **`halopsa-cli workday get`** - Use this to return a single instance of Workdays.<br>
				Requires authentication.
- **`halopsa-cli workday list`** - Use this to return multiple Workdays.<br>
				Requires authentication.

### workflow-target

Manage workflow target

- **`halopsa-cli workflow-target create`** - Create
- **`halopsa-cli workflow-target delete`** - Delete
- **`halopsa-cli workflow-target get`** - Get
- **`halopsa-cli workflow-target list`** - List

### workflows

Manage workflows

- **`halopsa-cli workflows create`** - Create
- **`halopsa-cli workflows delete`** - Delete
- **`halopsa-cli workflows get`** - Use this to return a single instance of FlowHeader.<br>
				Requires authentication.
- **`halopsa-cli workflows list`** - Use this to return multiple FlowHeader.<br>
				Requires authentication.

### workflowstep

Manage workflowstep

- **`halopsa-cli workflowstep`** - Use this to return multiple FlowDetail.<br>
				Requires authentication.

### xero-details

Manage xero details

- **`halopsa-cli xero-details create`** - Create
- **`halopsa-cli xero-details delete`** - Delete
- **`halopsa-cli xero-details get`** - Use this to return a single instance of XeroDetails.<br>
				Requires authentication.
- **`halopsa-cli xero-details list`** - Use this to return multiple XeroDetails.<br>
				Requires authentication.

### xtype-role

Manage xtype role

- **`halopsa-cli xtype-role`** - Use this to return multiple XTypeRole.<br>
				Requires authentication.

### zendesk

Manage zendesk

- **`halopsa-cli zendesk`** - List

### zoom

Manage zoom

- **`halopsa-cli zoom`** - Create


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
halopsa-cli actions list

# JSON for scripting and agents
halopsa-cli actions list --json

# Filter to specific fields
halopsa-cli actions list --json --select id,name,status

# Dry run  -  show the request without sending
halopsa-cli actions list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
halopsa-cli actions list --agent
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

## Runtime Endpoint

This CLI resolves endpoint placeholders at runtime, so one installed binary can target different tenants or API versions without regeneration.

Endpoint environment variables:
- `HALOPSA_TENANT` resolves `{tenant}`
- `HALOPSA_DOMAIN` resolves `{domain}`

Base URL: `https://{tenant}.{domain}/api`

## Health Check

```bash
halopsa-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/halo-pp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `HALOPSA_TENANT` | endpoint | Yes |  |
| `HALOPSA_DOMAIN` | endpoint | Yes |  |
| `HALOPSA_CLIENT_ID` | auth_flow_input | Yes | OAuth2 client ID from your Halo API application |
| `HALOPSA_CLIENT_SECRET` | auth_flow_input | Yes | Set during initial auth setup. |
| `HALOPSA_TENANT` | auth_flow_input | Yes | Your Halo tenant subdomain (e.g. "acme-msp" for acme-msp.halopsa.com) |
| `HALOPSA_DOMAIN` | auth_flow_input | No | Halo domain root: halopsa.com (default), haloitsm.com, or halocrm.com |
| `HALOPSA_SCOPE` | auth_flow_input | No | OAuth2 scope (defaults to "all"); set to a narrower scope when your API application requires it |
| `HALOPSA_TOKEN` | per_call | No | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `halopsa-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `halopsa-cli doctor` to check credentials
- Verify the environment variable is set: `echo $HALOPSA_CLIENT_ID`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every request**  -  Run `halopsa-cli doctor`  -  the token may have expired or your client_id lost the required scopes. Re-run `auth login`.
- **Empty list results that should have data**  -  Pass `--include-inactive` (clients/users) or `--include-deleted` (tickets)  -  Halo filters these by default. Verify with `halopsa-cli sql "SELECT COUNT(*) FROM tickets"`.
- **`sync` runs slowly on first call**  -  Initial sync pulls all 952 endpoints' top resources. Subsequent `sync` runs use `lastupdatedfrom` and are fast. Use `--only tickets,clients` to scope.
- **429 Too Many Requests during a burst**  -  The client auto-retries with exponential backoff. If it persists, lower `--concurrency` (default 4) on batch commands.
- **Tenant URL wrong / DNS error**  -  Confirm your Halo subdomain at https://<tenant>.halopsa.com (or .haloitsm.com / .halocrm.com). Pass `--tenant-domain haloitsm.com` to override the default `.halopsa.com`.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**homotechsual/HaloAPI**](https://github.com/homotechsual/HaloAPI)  -  PowerShell
- [**ssmanji89/haloapi-mcp-tools**](https://github.com/ssmanji89/haloapi-mcp-tools)  -  Python
- [**ssmanji89/halopsa-workflows-mcp**](https://github.com/ssmanji89/halopsa-workflows-mcp)  -  Python
- [**greenlighttec/pyhaloapi**](https://github.com/greenlighttec/pyhaloapi)  -  Python
- [**panoramicdata/HaloPsa.Api**](https://github.com/panoramicdata/HaloPsa.Api)  -  C#
- [**mspautomator/Halo_ServiceDesk**](https://github.com/mspautomator/Halo_ServiceDesk)  -  PowerShell
- [**amplify-msp/py-halo**](https://github.com/amplify-msp/py-halo)  -  Python
- [**lwhitelock/HaloPSA-Automation**](https://github.com/lwhitelock/HaloPSA-Automation)  -  PowerShell

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
