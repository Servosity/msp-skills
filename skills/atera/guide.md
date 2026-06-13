# Atera CLI

**Every Atera RMM + PSA endpoint, plus a local SQLite mirror that answers fleet-health, SLA, and book-of-business questions no single API call can.**

A single static API key unlocks full CRUD across agents, tickets, customers, contracts, alerts, devices, rates, and custom fields. The difference is the local store: sync once, then run offline cross-entity queries like `agents stale`, `tickets sla`, `customers book`, and `since` that every thin API wrapper leaves on the table.

## Install

The recommended path installs both the `atera-cli` binary and the `pp-atera` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install atera
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install atera --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install atera --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install atera --agent claude-code
npx -y @mvanhorn/printing-press-library install atera --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/atera/cmd/atera-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/atera-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install atera --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-atera --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-atera --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install atera --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/atera-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `ATERA_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/atera/cmd/atera-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "atera": {
      "command": "atera-mcp",
      "env": {
        "ATERA_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Atera uses one static API key passed in the `X-API-KEY` header. Generate it in Atera under Admin > API, then export it as `ATERA_API_KEY`. There is no OAuth and no per-user token  -  the same key is read-write for the whole account, so treat it as a secret. The API is HTTPS-only and rate-limited to 700 requests per minute; bulk `sync` paginates within that budget.

## Quick Start

```bash
# confirm your ATERA_API_KEY works and the API is reachable (set it via Atera > Admin > API)
atera-cli doctor

# pull agents, tickets, customers, contracts, alerts into the local store
atera-cli sync

# find machines that have gone dark
atera-cli agents stale --days 30 --agent

# rank open tickets by SLA breach risk
atera-cli tickets sla --agent

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Fleet health that compounds
- **`agents stale`**  -  Surface agents that have gone quiet  -  offline or not seen in N days  -  before the client calls to complain.

  _Reach for this when an agent asks which machines stopped reporting  -  it is a time-window query the live API cannot express._

  ```bash
  atera-cli agents stale --days 30 --agent
  ```
- **`agents inventory`**  -  Roll up OS and OS-type counts across the whole estate and flag end-of-life operating systems.

  _Use this to answer 'how many machines are still on an EOL OS' or 'what is the OS mix' in one call instead of paging every agent._

  ```bash
  atera-cli agents inventory --eol --agent
  ```
- **`alerts triage`**  -  Group open alerts by severity and customer, ranked by age, so the loudest problems surface first.

  _Use this to decide what to work first when the alert queue is noisy._

  ```bash
  atera-cli alerts triage --agent
  ```
- **`agents patch-status`**  -  Roll up missing-patch counts across the whole estate, ranked by agent and customer, from the per-device patch endpoints.

  _Reach for this when an agent asks which machines or customers are furthest behind on patches  -  no single API call answers it._

  ```bash
  atera-cli agents patch-status --limit 20 --agent
  ```
- **`agents noisy`**  -  Rank the machines and customers generating the most alerts over a window  -  the chronic problem devices.

  _Reach for this when an agent asks which devices keep alerting  -  alert-volume-by-machine over time, distinct from triaging the current queue._

  ```bash
  atera-cli agents noisy --days 7 --agent
  ```

### Service-desk intelligence
- **`tickets sla`**  -  Rank open tickets by how close they are to breaching first-response or resolution SLA.

  _Pick this for 'what's about to breach SLA'  -  it computes time-to-breach the API never returns._

  ```bash
  atera-cli tickets sla --agent
  ```
- **`tickets workload`**  -  Group open tickets and total logged duration by technician to spot who is overloaded.

  _Reach for this before reassigning work  -  it shows real open-ticket load per tech._

  ```bash
  atera-cli tickets workload --agent
  ```

### Book of business
- **`customers book`**  -  Per-customer rollup of agent count and contract mix (types + active/inactive) by joining customers, contracts, and agents.

  _Use this for account reviews  -  which customers are under contract vs. ad-hoc, and what each is worth._

  ```bash
  atera-cli customers book --agent
  ```
- **`customers coverage`**  -  Find under-contracted customers  -  managed agents with no active recurring or monitoring contract  -  before the renewal conversation.

  _Reach for this when an agent asks which customers are under-contracted or where revenue is leaking  -  it is the actionable gap filter, not the full rollup._

  ```bash
  atera-cli customers coverage --agent
  ```
- **`contracts expiring`**  -  See which contracts end inside a window, ranked by days-to-expiry and joined to the customer name.

  _Reach for this when an agent asks what is up for renewal  -  a time-window ranking the live API cannot express._

  ```bash
  atera-cli contracts expiring --days 60 --agent
  ```

### Time-windowed awareness
- **`since`**  -  Show what was created in a time window  -  new agents, new tickets, new alerts  -  across the synced estate.

  _Pick this for a stand-up summary of what was created across the whole account since yesterday._

  ```bash
  atera-cli since 24h --agent
  ```

## Recipes


### Narrow a large agent payload for an agent

```bash
atera-cli agents get --agent --select AgentName,OS,LastSeen,HardwareDisks.Size
```

Agent records are large and nested; `--select` with a dotted path keeps only the fields you need so agents do not burn context on full payloads.

### Daily SLA stand-up

```bash
atera-cli tickets sla --agent
```

Ranks open tickets by time-to-breach so the morning triage starts with what is most at risk.

### Find dark machines

```bash
atera-cli agents stale --days 14 --json
```

Lists agents whose last check-in is older than two weeks  -  the offline machines worth chasing.

### Account review

```bash
atera-cli customers book --agent
```

One per-customer rollup of agent count and contract mix for QBR prep.

### What moved overnight

```bash
atera-cli since 24h --agent
```

A one-shot summary of new agents, new tickets, and new alerts since yesterday.

### Find under-contracted customers before renewals

```bash
atera-cli customers coverage --agent
```

Joins agents to contracts per customer and surfaces managed endpoints with no active recurring/monitoring contract  -  the margin leak no Atera screen shows.

### Fleet patch compliance sweep

```bash
atera-cli agents patch-status --limit 20 --agent
```

Fans out the per-device available-patches endpoint across the synced estate and ranks machines by missing-patch count.

## Usage

Run `atera-cli --help` for the full command reference and flag list.

## Commands

### account

Manage account

- **`atera-cli account`** - Get info

### agents

Manage agents

- **`atera-cli agents delete`** - Deletes an agent. Requires the agent ID.
- **`atera-cli agents get`** - Returns a list of agents.
<br /> 
Please note that the property 'LastPatchManagementReceived' is no longer supported and will return a null reference.
- **`atera-cli agents get-by-customer`** - Returns agents for a specified customer. Requires the customer ID.
<br /> 
Please note that the property 'LastPatchManagementReceived' is no longer supported and will return a null reference.
- **`atera-cli agents get-by-machine-name`** - Returns agents installed on a specified machine. Requires the machine name.
<br /> 
Please note that the property 'LastPatchManagementReceived' is no longer supported and will return a null reference.
- **`atera-cli agents query-dto`** - Returns an agent. Requires the agent ID.
<br /> 
Please note that the property 'LastPatchManagementReceived' is no longer supported and will return a null reference.

### alerts

Manage alerts

- **`atera-cli alerts delete`** - Deletes a specified alert. Requires the alert ID.
- **`atera-cli alerts get`** - Returns a list of alerts.
- **`atera-cli alerts get-alertid`** - Returns an alert. Requires the alert ID.
- **`atera-cli alerts post`** - Creates a new alert. Requires the device's global unique identifier (GUID).
- **`atera-cli alerts resolve`** - Resolve a specified alert. Requires the alert ID.

### billing

Manage billing

- **`atera-cli billing get`** - Returns a list of invoices.
- **`atera-cli billing invoice-query-dto`** - Returns a specified invoice. Requires the invoice number.

### contacts

Manage contacts

- **`atera-cli contacts delete`** - Deletes the contact. Requires the contact ID.
- **`atera-cli contacts get`** - Accepts whole or partial emails / phone numbers. If partial, will return all contacts whose emails / phone numbers contain the input string.
<br />
Return Contact List
- **`atera-cli contacts get-contactid`** - Returns the specified contact. Requires the contact ID.
- **`atera-cli contacts post`** - Creates a new contact for an existing customer. Requires the customer ID or the customer name.
- **`atera-cli contacts put`** - Updates an existing contact. Requires the contact ID.
<br /><br /> 
The following fields are editable:
<br /><br /> 
First Name (Firstname)
<br />
Last Name (Lastname)
<br /> 
Job Title (JobTitle)
<br />
Is Contact Person (IsContactPerson)
<br />
In Ignore Mode (InIgnoreMode)
<br />
Phone (Phone)
- **`atera-cli contacts put-contactid`** - Updates an existing contact. Requires the contact ID.
<br /><br /> 
The following fields are editable:
<br /><br /> 
First Name (Firstname)
<br />
Last Name (Lastname)
<br /> 
Job Title (JobTitle)
<br />
Is Contact Person (IsContactPerson)
<br />
In Ignore Mode (InIgnoreMode)
<br />
Phone (Phone)

### contracts

Manage contracts

- **`atera-cli contracts get`** - Returns a list of contracts.
- **`atera-cli contracts get-by-customer`** - Returns a list of contracts. Requires the customer ID.
- **`atera-cli contracts post`** - Creates a new customer. Requires the customer name.
- **`atera-cli contracts query-dto`** - Returns a specified contract. Requires the contract ID.
- **`atera-cli contracts update`** - Updates an existing contract. Requires the contract ID.
- **`atera-cli contracts update-contractid`** - Updates an existing contract. Requires the contract ID.

### customers

Manage customers

- **`atera-cli customers delete`** - Deletes a specified customer. Requires the customer ID.
- **`atera-cli customers get`** - Returns a list of customers.
- **`atera-cli customers get-customerid`** - Returns a specified customer. Requires the customer ID.
- **`atera-cli customers post`** - Creates a new customer. Requires the customer name.
- **`atera-cli customers post-attachments`** - Requires the customer ID and attachment name, including the file extension. Requires attachment to be represented in Base64 encoding.
- **`atera-cli customers post-folders`** - Requires the folder name and customer ID.
Threshold profile ID is optional; if not included, the folder will inherit the threshold profile applied to the customer, if applicable.
- **`atera-cli customers put`** - Updates an existing customer. Requires the customer ID.
<br /><br /> 
The following fields are editable:
<br /><br /> 
Customer Name (CustomerName)
<br />
Business Number (BusinessNumber)
<br /> 
Domain (Domain)
<br />
Address (Address)
<br />
City (City)
<br />
State (State)
<br />
Country (Country)
<br />
Phone (Phone)
<br />
Fax (Fax)
<br />
Notes (Notes)
<br />
Links (Links)
<br />
Longitude (Longitude)
<br />
Latitude (Latitude)
<br />
Zip Code (ZipCodeStr)
- **`atera-cli customers put-customerid`** - Updates an existing customer. Requires the customer ID.
<br /><br /> 
The following fields are editable:
<br /><br /> 
Customer Name (CustomerName)
<br />
Business Number (BusinessNumber)
<br /> 
Domain (Domain)
<br />
Address (Address)
<br />
City (City)
<br />
State (State)
<br />
Country (Country)
<br />
Phone (Phone)
<br />
Fax (Fax)
<br />
Notes (Notes)
<br />
Links (Links)
<br />
Longitude (Longitude)
<br />
Latitude (Latitude)
<br />
Zip Code (ZipCodeStr)

### customvalues

Manage customvalues

- **`atera-cli customvalues custom-values-get`** - Returns a custom field value. Requires a custom value ID.
- **`atera-cli customvalues custom-values-get-agent-field`** - Returns a custom field value for a specified agent. Requires the agent ID and custom field name.
- **`atera-cli customvalues custom-values-get-agent-fields`** - Returns all custom field values for a specified agent. Requires the agent ID.
- **`atera-cli customvalues custom-values-get-contact-field`** - Returns a custom field value for a specified contact. Requires the contact ID and custom field name.
- **`atera-cli customvalues custom-values-get-contact-fields`** - Returns all custom field values for a specified contact. Requires the contact ID.
- **`atera-cli customvalues custom-values-get-contract-field`** - Returns a custom field value for a specified contract. Requires the contract ID and custom field name.
- **`atera-cli customvalues custom-values-get-contract-fields`** - Returns all custom field values for a specified contract. Requires the contract ID.
- **`atera-cli customvalues custom-values-get-custom-fields`** - Get list of custom field and value options
- **`atera-cli customvalues custom-values-get-customer-field`** - Returns a custom field value for a specified customer. Requires a customer ID and custom field name.
- **`atera-cli customvalues custom-values-get-customer-fields`** - Returns all custom field values for a specified customer. Requires the customer ID.
- **`atera-cli customvalues custom-values-get-generic-field`** - Returns a custom field value for a specified generic. Requires the generic ID and custom field name.
- **`atera-cli customvalues custom-values-get-generic-fields`** - Returns all custom field values for a specified generic device. Requires the generic device ID.
- **`atera-cli customvalues custom-values-get-httpfield`** - Returns a custom field value for a specified HTTP. Requires the HTTP ID and custom field name.
- **`atera-cli customvalues custom-values-get-httpfields`** - Returns all custom field values for a specified HTTP device. Requires the HTTP device ID.
- **`atera-cli customvalues custom-values-get-slafield`** - Returns a custom field value for a specified SLA. Requires the SLA ID and custom field name.
- **`atera-cli customvalues custom-values-get-slafields`** - Returns all custom field values for a specified SLA. Requires the SLA ID.
- **`atera-cli customvalues custom-values-get-snmpfield`** - Returns a custom field value for a specified SNMP. Requires the SNMP ID and custom field name.
- **`atera-cli customvalues custom-values-get-snmpfields`** - Returns all custom field values for a specified SNMP device. Requires the SNMP device ID.
- **`atera-cli customvalues custom-values-get-tcpfield`** - Returns a custom field value for a specified TCP. Requires the TCP ID and custom field name.
- **`atera-cli customvalues custom-values-get-tcpfields`** - Returns all custom field values for a specified TCP device. Requires the TCP device ID.
- **`atera-cli customvalues custom-values-get-ticket-field`** - Returns a custom field value for a specified ticket. Requires a ticket ID and a custom field name.
- **`atera-cli customvalues custom-values-get-ticket-fields`** - Returns all custom field values for a specified ticket. Requires the ticket ID.
- **`atera-cli customvalues custom-values-set-agent-field`** - Set value of custom field for specified Agent
- **`atera-cli customvalues custom-values-set-agent-field-agentfield`** - Set value of custom field for specified Agent
- **`atera-cli customvalues custom-values-set-agent-field-agentfield-2`** - Set value of custom field for specified Agent
- **`atera-cli customvalues custom-values-set-contact-field`** - Set value of custom field for specified Contact
- **`atera-cli customvalues custom-values-set-contact-field-contactfield`** - Set value of custom field for specified Contact
- **`atera-cli customvalues custom-values-set-contact-field-contactfield-2`** - Set value of custom field for specified Contact
- **`atera-cli customvalues custom-values-set-contract-field`** - Set value of custom field for specified Contract
- **`atera-cli customvalues custom-values-set-contract-field-contractfield`** - Set value of custom field for specified Contract
- **`atera-cli customvalues custom-values-set-contract-field-contractfield-2`** - Set value of custom field for specified Contract
- **`atera-cli customvalues custom-values-set-customer-field`** - Set value of custom field for specified Customer
- **`atera-cli customvalues custom-values-set-customer-field-customerfield`** - Set value of custom field for specified Customer
- **`atera-cli customvalues custom-values-set-customer-field-customerfield-2`** - Set value of custom field for specified Customer
- **`atera-cli customvalues custom-values-set-generic-field`** - Set value of custom field for specified Generic
- **`atera-cli customvalues custom-values-set-generic-field-genericfield`** - Set value of custom field for specified Generic
- **`atera-cli customvalues custom-values-set-generic-field-genericfield-2`** - Set value of custom field for specified Generic
- **`atera-cli customvalues custom-values-set-httpfield`** - Set value of custom field for specified HTTP
- **`atera-cli customvalues custom-values-set-httpfield-httpfield`** - Set value of custom field for specified HTTP
- **`atera-cli customvalues custom-values-set-httpfield-httpfield-2`** - Set value of custom field for specified HTTP
- **`atera-cli customvalues custom-values-set-slafield`** - Set value of custom field for specified SLA
- **`atera-cli customvalues custom-values-set-slafield-slafield`** - Set value of custom field for specified SLA
- **`atera-cli customvalues custom-values-set-slafield-slafield-2`** - Set value of custom field for specified SLA
- **`atera-cli customvalues custom-values-set-snmpfield`** - Set value of custom field for specified SNMP
- **`atera-cli customvalues custom-values-set-snmpfield-snmpfield`** - Set value of custom field for specified SNMP
- **`atera-cli customvalues custom-values-set-snmpfield-snmpfield-2`** - Set value of custom field for specified SNMP
- **`atera-cli customvalues custom-values-set-tcpfield`** - Set value of custom field for specified TCP
- **`atera-cli customvalues custom-values-set-tcpfield-tcpfield`** - Set value of custom field for specified TCP
- **`atera-cli customvalues custom-values-set-tcpfield-tcpfield-2`** - Set value of custom field for specified TCP
- **`atera-cli customvalues custom-values-set-ticket-field`** - Set custom field value for specified Ticket
- **`atera-cli customvalues custom-values-set-ticket-field-ticketfield`** - Set custom field value for specified Ticket
- **`atera-cli customvalues custom-values-set-ticket-field-ticketfield-2`** - Set custom field value for specified Ticket

### departments

Manage departments

- **`atera-cli departments delete`** - Deletes a specified department. Requires the department ID.
- **`atera-cli departments get`** - Returns a list of departments.
- **`atera-cli departments get-departmentid`** - Returns a specified department. Requires the department ID.
- **`atera-cli departments post`** - Creates a new department. Requires the department name.
- **`atera-cli departments put`** - Updates an existing department. Requires the department ID.
<br /><br /> 
The following fields are editable:
<br /><br /> 
Name: the name of the department
<br />
Description: a shor desciption of the department
- **`atera-cli departments put-departmentid`** - Updates an existing department. Requires the department ID.
<br /><br /> 
The following fields are editable:
<br /><br /> 
Name: the name of the department
<br />
Description: a shor desciption of the department

### devices

Manage devices

- **`atera-cli devices create-generic`** - Returns device ID and Device Guid.
- **`atera-cli devices create-http`** - Returns device ID and Device Guid.
- **`atera-cli devices create-snmp-v1-v2`** - Returns device ID and Device Guid.
- **`atera-cli devices create-snmp-v3`** - Returns device ID and Device Guid.
- **`atera-cli devices create-tcpdevice`** - Returns device ID and Device Guid.
- **`atera-cli devices delete`** - Deletes a Generic device. Requires the device ID.
- **`atera-cli devices delete-http`** - Deletes an HTTP device. Requires the device ID.
- **`atera-cli devices delete-snmp`** - Deletes an SNMP device. Requires the device ID.
- **`atera-cli devices delete-tcp`** - Deletes a TCP device. Requires the device ID.
- **`atera-cli devices get-generic`** - Returns a list of Generic devices.
- **`atera-cli devices get-generic-genericdevice`** - Returns a Generic device. Requires the device ID.
- **`atera-cli devices get-http`** - Returns an HTTP device. Requires the device ID.
- **`atera-cli devices get-httpdevices`** - Returns a list of HTTP devices.
- **`atera-cli devices get-snmpdevice`** - Returns an SNMP device. Requires the device ID.
- **`atera-cli devices get-snmpdevices`** - Returns a list of SNMP devices.
- **`atera-cli devices get-tcp`** - Returns a list of TCP devices.
- **`atera-cli devices get-tcp-tcpdevice`** - Returns a TCP device. Requires the device ID.

### knowledgebases

Manage knowledgebases

- **`atera-cli knowledgebases`** - Returns a list of articles.

### rates

Manage rates

- **`atera-cli rates delete-expense`** - Deletes a specified expense. Requires the expense ID.
- **`atera-cli rates delete-product`** - Deletes a specified product. Requires the product ID.
- **`atera-cli rates expense-query-dto`** - Returns a specified expense. Requires the expense ID.
- **`atera-cli rates get-expenses`** - Returns a list of expenses.
- **`atera-cli rates get-products`** - Returns a list of products.
- **`atera-cli rates post-expense`** - Creates a new expense. Requires an expense description.
- **`atera-cli rates post-product`** - Creates a new product. Requires a product description.
- **`atera-cli rates product-query-dto`** - Returns a specified product. Requires the product ID.
- **`atera-cli rates put-expense`** - Updates a specified expense. Requires the expense ID.
<br /><br /> 
The following fields are editable:
<br /><br /> 
Description
<br />
Category
<br /> 
Amount
<br />
SKU
<br />
Archived
- **`atera-cli rates put-product`** - Updates a specified product. Requires the product ID.
<br /><br /> 
The following fields are editable:
<br /><br /> 
Description
<br />
Category
<br /> 
Amount
<br />
SKU
<br />
Archived

### tickets

Manage tickets

- **`atera-cli tickets delete`** - Deletes the specified ticket. Requires the ticket ID.
- **`atera-cli tickets get`** - Returns a list of tickets.
- **`atera-cli tickets get-billable-durations`** - Returns the tickets (billable duration). Requires the ticket ID's.
- **`atera-cli tickets get-billable-workhours-site-duration`** - Returns a summary of the specified tickets on-site and off-site workhours duration
- **`atera-cli tickets get-by-last-modified-async`** - Returns a list of modified tickets.
- **`atera-cli tickets get-ticketid`** - Returns the specified ticket. Requires the ticket ID.
- **`atera-cli tickets post`** - Creates a new ticket. Requires the enduser contact ID, title, and description.
<br /><br /> 
If you wish to create a new enduser for the ticket then you need to provide:
<br /><br /> 
Contact first name (EndUserFirstName)
<br /> 
Contact last name (EndUserLastName)
<br /> 
Email (EndUserEmail)
- **`atera-cli tickets put`** - Updates an existing ticket. Requires the ticket ID.
<br /><br /> 
The following fields are editable:
<br /><br /> 
Ticket Title (TicketTitle)
<br />
Ticket Status (TicketStatus)
<br /> 
Ticket Type (TicketType)
<br />
Ticket Priority (TicketPriority)
<br />
Ticket Impact (TicketImpact)
<br />
Assigned Technician ID (TechnicianContactID)
- **`atera-cli tickets put-ticketid`** - Updates an existing ticket. Requires the ticket ID.
<br /><br /> 
The following fields are editable:
<br /><br /> 
Ticket Title (TicketTitle)
<br />
Ticket Status (TicketStatus)
<br /> 
Ticket Type (TicketType)
<br />
Ticket Priority (TicketPriority)
<br />
Ticket Impact (TicketImpact)
<br />
Assigned Technician ID (TechnicianContactID)
- **`atera-cli tickets track-status-modified`** - Returns a list of resolved and closed tickets.


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
atera-cli agents get

# JSON for scripting and agents
atera-cli agents get --json

# Filter to specific fields
atera-cli agents get --json --select id,name,status

# Dry run  -  show the request without sending
atera-cli agents get --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
atera-cli agents get --agent
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
atera-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/atera-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `ATERA_API_KEY` | per_call | No | Set to your API credential. |
| `ATERA_ACCOUNT_API` | per_call | No | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `atera-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `atera-cli doctor` to check credentials
- Verify the environment variable is set: `echo $ATERA_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call**  -  Set ATERA_API_KEY to a valid key from Atera > Admin > API; run `atera-cli doctor` to confirm.
- **429 / requests rejected under load**  -  Atera caps at 700 requests/minute  -  let `sync` pace itself rather than scripting tight loops; retry after a short backoff.
- **Transcendence commands return [] (empty)**  -  Run `atera-cli sync` first  -  `agents stale`, `tickets sla`, `customers book`, and `since` read the local store.
- **List command returns huge JSON**  -  Add `--select` with the fields you need (e.g. `--select AgentName,OS,LastSeen`) and `--agent` for compact agent-friendly output.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**atera-connector**](https://github.com/brianatalliance/atera-connector)  -  Python
- [**healdigital/atera-mcp-server**](https://github.com/healdigital/atera-mcp-server)  -  Python
- [**pYtera**](https://github.com/S1lvr/pYtera)  -  Python
- [**PSAtera**](https://github.com/davejlong/PSAtera)  -  PowerShell
- [**grandua/atera-mcp-server**](https://github.com/grandua/atera-mcp-server)  -  C#
- [**atera-client**](https://github.com/InoGo-Software/atera-client)  -  Python
- [**atera-api-php-client**](https://github.com/bluerocktel/atera-api-php-client)  -  PHP

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
