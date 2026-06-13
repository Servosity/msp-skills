---
name: atera
description: "Every Atera RMM + PSA endpoint, plus a local SQLite mirror that answers fleet-health, SLA, and book-of-business questions no single API call can. Trigger phrases: `which atera agents are offline`, `atera tickets about to breach sla`, `atera book of business`, `what changed in atera since yesterday`, `sync atera to my machine`, `use atera`, `run atera-cli`, `which customers are under-contracted`, `patch compliance across the fleet`, `what contracts are expiring`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Atera"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - atera-cli
    install:
      - kind: go
        bins: [atera-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/atera/cmd/atera-cli
---

# Atera  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `atera-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install atera --cli-only
   ```
2. Verify: `atera-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/atera/cmd/atera-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

A single static API key unlocks full CRUD across agents, tickets, customers, contracts, alerts, devices, rates, and custom fields. The difference is the local store: sync once, then run offline cross-entity queries like `agents stale`, `tickets sla`, `customers book`, and `since` that every thin API wrapper leaves on the table.

## When to Use This CLI

Reach for this CLI for any Atera RMM/PSA task an agent runs from the terminal: pulling or mutating agents, tickets, customers, contracts, alerts, devices, rates, and custom fields. It is the right choice when the question spans more than one entity or a time window  -  dark-agent sweeps, SLA-breach triage, technician load, book-of-business reviews, and change summaries  -  because those run against the local synced store instead of N paginated API calls.

## Unique Capabilities

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

## Command Reference

**account**  -  Manage account

- `atera-cli account`  -  Get info

**agents**  -  Manage agents

- `atera-cli agents delete`  -  Deletes an agent. Requires the agent ID.
- `atera-cli agents get`  -  Returns a list of agents.
- `atera-cli agents get-by-customer`  -  Returns agents for a specified customer. Requires the customer ID.
- `atera-cli agents get-by-machine-name`  -  Returns agents installed on a specified machine. Requires the machine name.
- `atera-cli agents query-dto`  -  Returns an agent. Requires the agent ID.

**alerts**  -  Manage alerts

- `atera-cli alerts delete`  -  Deletes a specified alert. Requires the alert ID.
- `atera-cli alerts get`  -  Returns a list of alerts.
- `atera-cli alerts get-alertid`  -  Returns an alert. Requires the alert ID.
- `atera-cli alerts post`  -  Creates a new alert. Requires the device's global unique identifier (GUID).
- `atera-cli alerts resolve`  -  Resolve a specified alert. Requires the alert ID.

**billing**  -  Manage billing

- `atera-cli billing get`  -  Returns a list of invoices.
- `atera-cli billing invoice-query-dto`  -  Returns a specified invoice. Requires the invoice number.

**contacts**  -  Manage contacts

- `atera-cli contacts delete`  -  Deletes the contact. Requires the contact ID.
- `atera-cli contacts get`  -  Accepts whole or partial emails / phone numbers.
- `atera-cli contacts get-contactid`  -  Returns the specified contact. Requires the contact ID.
- `atera-cli contacts post`  -  Creates a new contact for an existing customer. Requires the customer ID or the customer name.
- `atera-cli contacts put`  -  Updates an existing contact. Requires the contact ID.
- `atera-cli contacts put-contactid`  -  Updates an existing contact. Requires the contact ID.

**contracts**  -  Manage contracts

- `atera-cli contracts get`  -  Returns a list of contracts.
- `atera-cli contracts get-by-customer`  -  Returns a list of contracts. Requires the customer ID.
- `atera-cli contracts post`  -  Creates a new customer. Requires the customer name.
- `atera-cli contracts query-dto`  -  Returns a specified contract. Requires the contract ID.
- `atera-cli contracts update`  -  Updates an existing contract. Requires the contract ID.
- `atera-cli contracts update-contractid`  -  Updates an existing contract. Requires the contract ID.

**customers**  -  Manage customers

- `atera-cli customers delete`  -  Deletes a specified customer. Requires the customer ID.
- `atera-cli customers get`  -  Returns a list of customers.
- `atera-cli customers get-customerid`  -  Returns a specified customer. Requires the customer ID.
- `atera-cli customers post`  -  Creates a new customer. Requires the customer name.
- `atera-cli customers post-attachments`  -  Requires the customer ID and attachment name, including the file extension.
- `atera-cli customers post-folders`  -  Requires the folder name and customer ID.
- `atera-cli customers put`  -  Updates an existing customer. Requires the customer ID.
- `atera-cli customers put-customerid`  -  Updates an existing customer. Requires the customer ID.

**customvalues**  -  Manage customvalues

- `atera-cli customvalues custom-values-get`  -  Returns a custom field value. Requires a custom value ID.
- `atera-cli customvalues custom-values-get-agent-field`  -  Returns a custom field value for a specified agent. Requires the agent ID and custom field name.
- `atera-cli customvalues custom-values-get-agent-fields`  -  Returns all custom field values for a specified agent. Requires the agent ID.
- `atera-cli customvalues custom-values-get-contact-field`  -  Returns a custom field value for a specified contact. Requires the contact ID and custom field name.
- `atera-cli customvalues custom-values-get-contact-fields`  -  Returns all custom field values for a specified contact. Requires the contact ID.
- `atera-cli customvalues custom-values-get-contract-field`  -  Returns a custom field value for a specified contract. Requires the contract ID and custom field name.
- `atera-cli customvalues custom-values-get-contract-fields`  -  Returns all custom field values for a specified contract. Requires the contract ID.
- `atera-cli customvalues custom-values-get-custom-fields`  -  Get list of custom field and value options
- `atera-cli customvalues custom-values-get-customer-field`  -  Returns a custom field value for a specified customer. Requires a customer ID and custom field name.
- `atera-cli customvalues custom-values-get-customer-fields`  -  Returns all custom field values for a specified customer. Requires the customer ID.
- `atera-cli customvalues custom-values-get-generic-field`  -  Returns a custom field value for a specified generic. Requires the generic ID and custom field name.
- `atera-cli customvalues custom-values-get-generic-fields`  -  Returns all custom field values for a specified generic device. Requires the generic device ID.
- `atera-cli customvalues custom-values-get-httpfield`  -  Returns a custom field value for a specified HTTP. Requires the HTTP ID and custom field name.
- `atera-cli customvalues custom-values-get-httpfields`  -  Returns all custom field values for a specified HTTP device. Requires the HTTP device ID.
- `atera-cli customvalues custom-values-get-slafield`  -  Returns a custom field value for a specified SLA. Requires the SLA ID and custom field name.
- `atera-cli customvalues custom-values-get-slafields`  -  Returns all custom field values for a specified SLA. Requires the SLA ID.
- `atera-cli customvalues custom-values-get-snmpfield`  -  Returns a custom field value for a specified SNMP. Requires the SNMP ID and custom field name.
- `atera-cli customvalues custom-values-get-snmpfields`  -  Returns all custom field values for a specified SNMP device. Requires the SNMP device ID.
- `atera-cli customvalues custom-values-get-tcpfield`  -  Returns a custom field value for a specified TCP. Requires the TCP ID and custom field name.
- `atera-cli customvalues custom-values-get-tcpfields`  -  Returns all custom field values for a specified TCP device. Requires the TCP device ID.
- `atera-cli customvalues custom-values-get-ticket-field`  -  Returns a custom field value for a specified ticket. Requires a ticket ID and a custom field name.
- `atera-cli customvalues custom-values-get-ticket-fields`  -  Returns all custom field values for a specified ticket. Requires the ticket ID.
- `atera-cli customvalues custom-values-set-agent-field`  -  Set value of custom field for specified Agent
- `atera-cli customvalues custom-values-set-agent-field-agentfield`  -  Set value of custom field for specified Agent
- `atera-cli customvalues custom-values-set-agent-field-agentfield-2`  -  Set value of custom field for specified Agent
- `atera-cli customvalues custom-values-set-contact-field`  -  Set value of custom field for specified Contact
- `atera-cli customvalues custom-values-set-contact-field-contactfield`  -  Set value of custom field for specified Contact
- `atera-cli customvalues custom-values-set-contact-field-contactfield-2`  -  Set value of custom field for specified Contact
- `atera-cli customvalues custom-values-set-contract-field`  -  Set value of custom field for specified Contract
- `atera-cli customvalues custom-values-set-contract-field-contractfield`  -  Set value of custom field for specified Contract
- `atera-cli customvalues custom-values-set-contract-field-contractfield-2`  -  Set value of custom field for specified Contract
- `atera-cli customvalues custom-values-set-customer-field`  -  Set value of custom field for specified Customer
- `atera-cli customvalues custom-values-set-customer-field-customerfield`  -  Set value of custom field for specified Customer
- `atera-cli customvalues custom-values-set-customer-field-customerfield-2`  -  Set value of custom field for specified Customer
- `atera-cli customvalues custom-values-set-generic-field`  -  Set value of custom field for specified Generic
- `atera-cli customvalues custom-values-set-generic-field-genericfield`  -  Set value of custom field for specified Generic
- `atera-cli customvalues custom-values-set-generic-field-genericfield-2`  -  Set value of custom field for specified Generic
- `atera-cli customvalues custom-values-set-httpfield`  -  Set value of custom field for specified HTTP
- `atera-cli customvalues custom-values-set-httpfield-httpfield`  -  Set value of custom field for specified HTTP
- `atera-cli customvalues custom-values-set-httpfield-httpfield-2`  -  Set value of custom field for specified HTTP
- `atera-cli customvalues custom-values-set-slafield`  -  Set value of custom field for specified SLA
- `atera-cli customvalues custom-values-set-slafield-slafield`  -  Set value of custom field for specified SLA
- `atera-cli customvalues custom-values-set-slafield-slafield-2`  -  Set value of custom field for specified SLA
- `atera-cli customvalues custom-values-set-snmpfield`  -  Set value of custom field for specified SNMP
- `atera-cli customvalues custom-values-set-snmpfield-snmpfield`  -  Set value of custom field for specified SNMP
- `atera-cli customvalues custom-values-set-snmpfield-snmpfield-2`  -  Set value of custom field for specified SNMP
- `atera-cli customvalues custom-values-set-tcpfield`  -  Set value of custom field for specified TCP
- `atera-cli customvalues custom-values-set-tcpfield-tcpfield`  -  Set value of custom field for specified TCP
- `atera-cli customvalues custom-values-set-tcpfield-tcpfield-2`  -  Set value of custom field for specified TCP
- `atera-cli customvalues custom-values-set-ticket-field`  -  Set custom field value for specified Ticket
- `atera-cli customvalues custom-values-set-ticket-field-ticketfield`  -  Set custom field value for specified Ticket
- `atera-cli customvalues custom-values-set-ticket-field-ticketfield-2`  -  Set custom field value for specified Ticket

**departments**  -  Manage departments

- `atera-cli departments delete`  -  Deletes a specified department. Requires the department ID.
- `atera-cli departments get`  -  Returns a list of departments.
- `atera-cli departments get-departmentid`  -  Returns a specified department. Requires the department ID.
- `atera-cli departments post`  -  Creates a new department. Requires the department name.
- `atera-cli departments put`  -  Updates an existing department. Requires the department ID.
- `atera-cli departments put-departmentid`  -  Updates an existing department. Requires the department ID.

**devices**  -  Manage devices

- `atera-cli devices create-generic`  -  Returns device ID and Device Guid.
- `atera-cli devices create-http`  -  Returns device ID and Device Guid.
- `atera-cli devices create-snmp-v1-v2`  -  Returns device ID and Device Guid.
- `atera-cli devices create-snmp-v3`  -  Returns device ID and Device Guid.
- `atera-cli devices create-tcpdevice`  -  Returns device ID and Device Guid.
- `atera-cli devices delete`  -  Deletes a Generic device. Requires the device ID.
- `atera-cli devices delete-http`  -  Deletes an HTTP device. Requires the device ID.
- `atera-cli devices delete-snmp`  -  Deletes an SNMP device. Requires the device ID.
- `atera-cli devices delete-tcp`  -  Deletes a TCP device. Requires the device ID.
- `atera-cli devices get-generic`  -  Returns a list of Generic devices.
- `atera-cli devices get-generic-genericdevice`  -  Returns a Generic device. Requires the device ID.
- `atera-cli devices get-http`  -  Returns an HTTP device. Requires the device ID.
- `atera-cli devices get-httpdevices`  -  Returns a list of HTTP devices.
- `atera-cli devices get-snmpdevice`  -  Returns an SNMP device. Requires the device ID.
- `atera-cli devices get-snmpdevices`  -  Returns a list of SNMP devices.
- `atera-cli devices get-tcp`  -  Returns a list of TCP devices.
- `atera-cli devices get-tcp-tcpdevice`  -  Returns a TCP device. Requires the device ID.

**knowledgebases**  -  Manage knowledgebases

- `atera-cli knowledgebases`  -  Returns a list of articles.

**rates**  -  Manage rates

- `atera-cli rates delete-expense`  -  Deletes a specified expense. Requires the expense ID.
- `atera-cli rates delete-product`  -  Deletes a specified product. Requires the product ID.
- `atera-cli rates expense-query-dto`  -  Returns a specified expense. Requires the expense ID.
- `atera-cli rates get-expenses`  -  Returns a list of expenses.
- `atera-cli rates get-products`  -  Returns a list of products.
- `atera-cli rates post-expense`  -  Creates a new expense. Requires an expense description.
- `atera-cli rates post-product`  -  Creates a new product. Requires a product description.
- `atera-cli rates product-query-dto`  -  Returns a specified product. Requires the product ID.
- `atera-cli rates put-expense`  -  Updates a specified expense. Requires the expense ID.
- `atera-cli rates put-product`  -  Updates a specified product. Requires the product ID.

**tickets**  -  Manage tickets

- `atera-cli tickets delete`  -  Deletes the specified ticket. Requires the ticket ID.
- `atera-cli tickets get`  -  Returns a list of tickets.
- `atera-cli tickets get-billable-durations`  -  Returns the tickets (billable duration). Requires the ticket ID's.
- `atera-cli tickets get-billable-workhours-site-duration`  -  Returns a summary of the specified tickets on-site and off-site workhours duration
- `atera-cli tickets get-by-last-modified-async`  -  Returns a list of modified tickets.
- `atera-cli tickets get-ticketid`  -  Returns the specified ticket. Requires the ticket ID.
- `atera-cli tickets post`  -  Creates a new ticket. Requires the enduser contact ID, title, and description.
- `atera-cli tickets put`  -  Updates an existing ticket. Requires the ticket ID.
- `atera-cli tickets put-ticketid`  -  Updates an existing ticket. Requires the ticket ID.
- `atera-cli tickets track-status-modified`  -  Returns a list of resolved and closed tickets.


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
atera-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

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

## Auth Setup

Atera uses one static API key passed in the `X-API-KEY` header. Generate it in Atera under Admin > API, then export it as `ATERA_API_KEY`. There is no OAuth and no per-user token  -  the same key is read-write for the whole account, so treat it as a secret. The API is HTTPS-only and rate-limited to 700 requests per minute; bulk `sync` paginates within that budget.

Run `atera-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  atera-cli agents get --agent --select id,name,status
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
atera-cli feedback "the --since flag is inclusive but docs say exclusive"
atera-cli feedback --stdin < notes.txt
atera-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/atera-cli/feedback.jsonl`. They are never POSTed unless `ATERA_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `ATERA_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled agent calls the same command every run with the same configuration - HeyGen's "Beacon" pattern.

```
atera-cli profile save briefing --json
atera-cli --profile briefing agents get
atera-cli profile list --json
atera-cli profile show briefing
atera-cli profile delete briefing --yes
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
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `atera-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/atera/cmd/atera-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add atera-mcp -- atera-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which atera-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   atera-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `atera-cli <command> --help`.
