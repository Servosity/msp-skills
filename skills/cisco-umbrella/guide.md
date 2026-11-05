# Cisco Umbrella CLI

**Every Cisco Umbrella v2 API call, plus MSP child-org switching, domain verdicts, list diffs, and fleet health.**

Covers deployments, policies, reports, Investigate, admin, and the provider and managed-provider consoles from Cisco's official OpenAPI. One parent console key acts on any customer org with --org. hygiene reads the local store (sync first); verdict, destination-lists diff, allowlist audit, fleet health, and report-card call the API live.

Learn more at [Cisco Umbrella](https://developer.cisco.com/docs/cloud-security/).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `cisco-umbrella-cli` and `cisco-umbrella-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-umbrella/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-umbrella/install.ps1 | iex
   ```
3. Verify: `cisco-umbrella-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=cisco-umbrella). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/cisco-umbrella --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/cisco-umbrella --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `cisco-umbrella-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the cisco-umbrella skill from https://github.com/Servosity/msp-skills/tree/main/skills/cisco-umbrella. The skill defines how its required CLI (`cisco-umbrella-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=cisco-umbrella).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `CISCO_UMBRELLA_API_KEY` (API Key) and `CISCO_UMBRELLA_API_SECRET` (Key Secret) when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-umbrella/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-umbrella/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "cisco-umbrella": {
      "command": "cisco-umbrella-mcp",
      "env": {
        "CISCO_UMBRELLA_API_KEY": "<your-key>",
        "CISCO_UMBRELLA_API_SECRET": "<your-key-secret>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`cisco-umbrella-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `CISCO_UMBRELLA_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `CISCO_UMBRELLA_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export CISCO_UMBRELLA_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
cisco-umbrella-mcp --transport http
```

Set `CISCO_UMBRELLA_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

## Authentication

Create an Umbrella API key in the dashboard (Admin > API Keys, or Console Settings > API Keys in an MSP or Multi-org console) and set CISCO_UMBRELLA_API_KEY and CISCO_UMBRELLA_API_SECRET. The CLI exchanges them for a one-hour bearer token at https://api.umbrella.com/auth/v2/token. To act on one customer from a parent console key, set CISCO_UMBRELLA_ORG_ID (or pass --org); the token request then carries X-Umbrella-OrgId and is scoped to that child org. Each key only reaches the scopes it was created with; a missing scope returns 403.

## Quick Start

```bash
# Shows what doctor would check without touching the network; run 'doctor' to test the key for real.
cisco-umbrella-cli doctor --dry-run

# First live read; confirms the key has the deployments scope.
cisco-umbrella-cli roaming-computers list --agent

# Builds the local store that hygiene reads.
cisco-umbrella-cli sync --resources roaming_computers,virtual_appliances,tunnels-tunnels-state,networks

# Category, risk, and list membership for one domain.
cisco-umbrella-cli verdict example.com --agent

```

## Unique Features

### Triage and policy
- **`verdict`**  -  One answer for a domain: category, risk score, security status, every destination list that blocks or allows it (including parent-domain entries), and who resolved it recently.

  _Reach for this when a ticket or IOC asks 'is this domain blocked, how risky is it, and who hit it'. Needs the Policies, Investigate, and Reports scopes; a 403 skips that section, so check lists_checked, investigate_checked, and notes before calling a domain clean._

  ```bash
  cisco-umbrella-cli verdict login.example.com --agent
  ```
- **`destination-lists diff`**  -  Reconcile a destination list against a source-of-truth file: see what to add and remove, then apply it in API-sized chunks. --apply also removes entries absent from the file unless --no-remove.

  _Use this before any bulk block/allow change so the push is a reviewed diff, not a blind overwrite. It prints the plan unless --apply is given._

  ```bash
  cisco-umbrella-cli destination-lists diff 1234567 --file blocklist.txt --agent
  ```
- **`allowlist audit`**  -  Find allow-list entries that are now categorized as malicious, entries that conflict with a block list, and allows added in a recent window.

  _Catches allow-list rot that no dashboard view surfaces. Needs the Policies and Investigate scopes; categorization_checked is false when the Investigate check was skipped._

  ```bash
  cisco-umbrella-cli allowlist audit --since 7d --agent
  ```

### MSP fleet operations
- **`fleet health`**  -  One row per managed customer: seats, an issues count, and counts of off, uninstalled, stale, and SWG-unprotected roaming computers, unhealthy virtual appliances, and down tunnels. Customers that fail are listed under fetch_failures.

  _The MSP weekly sweep in one call. Run it from an MSP, Multi-org, or provider parent console key with the admin customers scope, without --org. It mints one child-org token per customer, starting customers 3s apart; a 403 section is listed under the row's skipped field._

  ```bash
  cisco-umbrella-cli fleet health --stale-days 7 --agent
  ```
- **`hygiene`**  -  Device-level findings in one org: stale or offline roaming computers, unhealthy virtual appliances, down tunnels, and unverified networks, each tagged with a kind.

  _Use after fleet health flags a customer, or for a weekly hygiene pass. It reads the local store: sync roaming_computers, virtual_appliances, tunnels-tunnels-state, and networks first (with the same --org); an empty store returns no findings plus a note._

  ```bash
  cisco-umbrella-cli hygiene --stale-days 14 --agent
  ```
- **`report-card`**  -  A customer's security summary, top threat types, blocked categories, and deployment status for a window, compared with the prior window. --csv emits the summary rows only.

  _QBR-ready numbers with a trend in one command. From a parent console key pass --org <customerId>; without it the card covers the key's own org._

  ```bash
  cisco-umbrella-cli report-card --org 8675309 --from -30days --agent
  ```

## Recipes

### Triage a domain from a ticket

```bash
cisco-umbrella-cli verdict login.example.com --agent
```

Category, risk score, matching destination lists, and recent resolvers in one JSON document.

### Block one domain

```bash
cisco-umbrella-cli destination-lists add-destinations 1234567 --body-json '[{"destination":"bad.example.com","comment":"phishing ticket"}]'
```

Adds one entry to a block list (list IDs come from destination-lists list); use diff --no-remove --apply for bulk adds.

### Narrow a roaming computer list

```bash
cisco-umbrella-cli roaming-computers list --agent --select name,status,lastSync
```

Keeps only the fields an agent needs from a large device list.

### Weekly MSP sweep

```bash
cisco-umbrella-cli fleet health --stale-days 7 --agent
```

One row per customer with an issues count; customers that fail land under fetch_failures. Use a parent console key and no --org.

### Reconcile a blocklist file

```bash
cisco-umbrella-cli destination-lists diff 1234567 --file blocklist.txt --agent
```

Prints the add/remove plan. Rerun with --apply to make it; --apply also removes entries absent from the file unless --no-remove.

## Usage

Run `cisco-umbrella-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `CISCO_UMBRELLA_CONFIG_DIR`, `CISCO_UMBRELLA_DATA_DIR`, `CISCO_UMBRELLA_STATE_DIR`, or `CISCO_UMBRELLA_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `CISCO_UMBRELLA_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export CISCO_UMBRELLA_HOME=/srv/cisco-umbrella
cisco-umbrella-cli doctor
```

On Windows (PowerShell):

```powershell
$env:CISCO_UMBRELLA_HOME = "C:\ProgramData\cisco-umbrella"
cisco-umbrella-cli doctor
```

Under `CISCO_UMBRELLA_HOME=/srv/cisco-umbrella`, the four dirs resolve to `/srv/cisco-umbrella/config`, `/srv/cisco-umbrella/data`, `/srv/cisco-umbrella/state`, and `/srv/cisco-umbrella/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:CISCO_UMBRELLA_HOME = "C:\srv\cisco-umbrella"
cisco-umbrella-cli doctor
```

Under `$env:CISCO_UMBRELLA_HOME = "C:\srv\cisco-umbrella"`, the four dirs resolve to `C:\srv\cisco-umbrella\config`, `C:\srv\cisco-umbrella\data`, `C:\srv\cisco-umbrella\state`, and `C:\srv\cisco-umbrella\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "cisco-umbrella": {
      "command": "cisco-umbrella-mcp",
      "env": {
        "CISCO_UMBRELLA_HOME": "/srv/cisco-umbrella"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `CISCO_UMBRELLA_DATA_DIR` overrides an explicit `--home` for that kind. Use `CISCO_UMBRELLA_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `CISCO_UMBRELLA_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `cisco-umbrella-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. API responses are not cached. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `cisco-umbrella-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

Credentials stay out of everything but `credentials.toml` and `config.toml`. Any API call with environment credentials saves the key, the secret and the bearer token to `credentials.toml`: owner-only, not encrypted. Later runs use them after the variables are unset, until `auth logout` deletes the file; while the variables stay set, the next API call saves them again. A key named on the run itself (`CISCO_UMBRELLA_API_KEY`, or the config file the run loads) wins over a different key, bearer or `auth set-token` token saved by an earlier run, so naming another customer's key reaches that customer. Output that repeats a credential the program holds, plain or encoded, is replaced whole by `[credkit: withheld from <sink>: it carried a credential this program holds]`, where `<sink>` names where it was going; a write to a file, the store or the journal that repeats one is refused. Credential-named fields in API responses are replaced by `[redacted]` first. Umbrella returns a newly generated secret only once, and this program withholds it, so the only copy would be lost: `api-keys create`, `api-keys refresh`, `s3-bucket-keys`, `tunnels add` with no secret of your own, and `tunnels update-credentials` with auto-rotate or with no secret of your own are refused before anything is sent. Do them in the Umbrella dashboard, where the secret is shown. A credential goes only over HTTPS to the configured API base URL or `https://api.umbrella.com`; a token URL on any other host is refused.

- **Erase it:** `cisco-umbrella-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept; `auth logout` deletes `credentials.toml`.
- **Run without a local copy:** set `CISCO_UMBRELLA_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `CISCO_UMBRELLA_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### activity

Manage activity

- **`cisco-umbrella-cli activity get-activities`** - List all activities (dns/proxy/firewall/intrusion) within the timeframe.
**Note:** The IP activity report is not available.

**Access Scope:** Reports > Granular Events > Read-Only
- **`cisco-umbrella-cli activity get-amp-retrospective`** - List all AMP retrospective activity within the timeframe.

**Access Scope:** Reports > Granular Events > Read-Only
- **`cisco-umbrella-cli activity get-dns`** - List all DNS entries within the timeframe.

**Access Scope:** Reports > Granular Events > Read-Only
- **`cisco-umbrella-cli activity get-firewall`** - List all firewall activity within the timeframe.

**Access Scope:** Reports > Granular Events > Read-Only
- **`cisco-umbrella-cli activity get-intrusion`** - List all Intrusion Prevention System (IPS) activity within the timeframe.

**Access Scope:** Reports > Granular Events > Read-Only
- **`cisco-umbrella-cli activity get-ip`** - (Deprecated) List all IP activity within the timeframe.

**Access Scope:** Reports > Granular Events > Read-Only
- **`cisco-umbrella-cli activity get-proxy`** - List all proxy entries within the timeframe.

**Access Scope:** Reports > Granular Events > Read-Only

### api_keys

Manage api keys

- **`cisco-umbrella-cli api-keys create`** - Create an API key with certain scopes, name, description, allowed IP addresses and CIDR blocks, and expiration. Refused here: Umbrella shows the new secret only once and this program withholds it, so do this in the Umbrella dashboard.
The `description` and `allowedIPs` fields are optional.
- **`cisco-umbrella-cli api-keys delete`** - Delete an API key.
- **`cisco-umbrella-cli api-keys get`** - Get an API key.
- **`cisco-umbrella-cli api-keys list`** - List the API keys created by your organization.
- **`cisco-umbrella-cli api-keys refresh`** - Refresh the API key and secret. Refused here: Umbrella shows the new secret only once and this program withholds it, so do this in the Umbrella dashboard.
- **`cisco-umbrella-cli api-keys update`** - Update the name, description, scopes, and allowed IPs for an API key.

### api_usage

Manage api usage

- **`cisco-umbrella-cli api-usage get-keys`** - Get the API key usage information, including the total number of API requests within a specific time period.
- **`cisco-umbrella-cli api-usage get-requests`** - Get the information about the API requests for the organization within a specific time period, including
the total number of API requests for the type of client program.
- **`cisco-umbrella-cli api-usage get-responses`** - Get the information about the API responses for the organization within a specific time period, including
the total number of API responses and the HTTP status codes.
- **`cisco-umbrella-cli api-usage get-summary`** - Get the total number API requests, and the counts of the successful and failed API requests within a specific time period.

### app_discovery

Manage app discovery

- **`cisco-umbrella-cli app-discovery get-application`** - Get an application by ID.
- **`cisco-umbrella-cli app-discovery get-application-attributes`** - List all attributes for the application.
- **`cisco-umbrella-cli app-discovery get-application-identities`** - List all identities for the application.
- **`cisco-umbrella-cli app-discovery get-application-risk`** - Get the risk for the application.
- **`cisco-umbrella-cli app-discovery get-applications-info`** - List the information about the applications.
- **`cisco-umbrella-cli app-discovery get-protocol`** - Get protocol by ID.
- **`cisco-umbrella-cli app-discovery get-protocol-identities`** - List identities for a specific protocol.
- **`cisco-umbrella-cli app-discovery list-application-categories`** - List all application categories.
- **`cisco-umbrella-cli app-discovery list-applications`** - List all discovered applications.
- **`cisco-umbrella-cli app-discovery list-protocols`** - List all protocols.
- **`cisco-umbrella-cli app-discovery patch-applications`** - Update the labels for the applications.
- **`cisco-umbrella-cli app-discovery update-application`** - Update the label for the application. Provide an application label in the request body.

### application_lists

Manage application lists

- **`cisco-umbrella-cli application-lists create`** - Create an application list for the organization.
- **`cisco-umbrella-cli application-lists delete`** - Delete the application list in the organization.
- **`cisco-umbrella-cli application-lists get`** - Get the properties of the application list.
- **`cisco-umbrella-cli application-lists get-applications-usage`** - Get the list of application usage information for the application lists.
- **`cisco-umbrella-cli application-lists list`** - Get the application lists and the number of application lists in the organization.
- **`cisco-umbrella-cli application-lists update`** - Update the properties of the application list.

### console_config

Manage console config

- **`cisco-umbrella-cli console-config create-cname`** - Add the cname information for the service providers console.
- **`cisco-umbrella-cli console-config create-contact`** - Create a contact for the service providers console.
- **`cisco-umbrella-cli console-config create-logo`** - Create a logo for the service providers console.
- **`cisco-umbrella-cli console-config delete-cname`** - Delete a cname for the service providers console.
- **`cisco-umbrella-cli console-config delete-contact`** - Delete a contact for the service providers console.
- **`cisco-umbrella-cli console-config delete-logo`** - Delete a logo for the service providers console.
- **`cisco-umbrella-cli console-config get-cname`** - Get the cname information for the service providers console.
- **`cisco-umbrella-cli console-config get-cnames`** - List the cname information for the service providers console.
- **`cisco-umbrella-cli console-config get-contact`** - Get a contact for the service providers console.
- **`cisco-umbrella-cli console-config get-contacts`** - List the contacts for the service providers console.
- **`cisco-umbrella-cli console-config get-logo`** - Get a logo for the service providers console.
- **`cisco-umbrella-cli console-config get-logos`** - Get the logos for the service providers console.
- **`cisco-umbrella-cli console-config update-cname`** - Update the cname information for the service providers console.
- **`cisco-umbrella-cli console-config update-contact`** - Update a contact for service providers console.
- **`cisco-umbrella-cli console-config update-logo`** - Update a logo for the service providers console.

### datacenters

Manage datacenters

- **`cisco-umbrella-cli datacenters`** - List the information about the IPsec-enabled data centers.
The data center information includes the IP address and location details.

### destination_lists

Manage destination lists

- **`cisco-umbrella-cli destination-lists add-destinations`** - Add destinations to a destination list.
- **`cisco-umbrella-cli destination-lists create`** - Create a destination list in your organization.
- **`cisco-umbrella-cli destination-lists delete`** - Delete a destination list from your organization.
- **`cisco-umbrella-cli destination-lists get`** - Get a destination list.
- **`cisco-umbrella-cli destination-lists list`** - Get the destination lists in your organization.
- **`cisco-umbrella-cli destination-lists list-destinations`** - Get destinations in a destination list.
- **`cisco-umbrella-cli destination-lists remove-destinations`** - Remove destinations from the destination list.

**Note:** Accepts a list that contains no more than 500 destination IDs.

You can retrieve a list of the destinations in the destination list through the GET `/destinationlists/{destinationListId}/destinations` operation.
Then, to remove destinations in a destination list, provide a list of destination IDs in the request body of the
DELETE `/destinationlists/{destinationListId}/destinations/remove` operation.
- **`cisco-umbrella-cli destination-lists update`** - Update a destination list in your organization.

### internal_domains

Manage internal domains

- **`cisco-umbrella-cli internal-domains create`** - Create an internal domain. If you do not assign a list of sites to the internal domain, the internal domain
is associated with all sites in the organization.
- **`cisco-umbrella-cli internal-domains delete`** - Delete an internal domain.
- **`cisco-umbrella-cli internal-domains get`** - Get an internal domain.
- **`cisco-umbrella-cli internal-domains list`** - List the internal domains.
- **`cisco-umbrella-cli internal-domains update`** - Update an internal domain.

### internal_networks

Manage internal networks

- **`cisco-umbrella-cli internal-networks create`** - Create an internal network.
- **`cisco-umbrella-cli internal-networks delete`** - Delete an internal network.
- **`cisco-umbrella-cli internal-networks get`** - Get an internal network.
- **`cisco-umbrella-cli internal-networks list`** - List the internal networks.
- **`cisco-umbrella-cli internal-networks list-policies`** - List the policies for an internal network.
- **`cisco-umbrella-cli internal-networks update`** - Update an internal network.

### investigate

Manage investigate

- **`cisco-umbrella-cli investigate get-bgp-routes-asn`** - A response to a valid ASN returns an array of hash references.
Each hash reference contains two keys: `geo` and `cidr`.
Geo is a hash reference with the country name and country code
(the code corresponds to the country code list for ISO-3166-1 alpha-2).
CIDR contains the IP prefix for this ASN.
- **`cisco-umbrella-cli investigate get-bgp-routes-ip`** - This endpoint provides data about ASN and IP relationships,
showing how IP addresses are related to each other and to the regional registries.
You can find out more about the IP space associated with an AS
and correlate BGP routing information between AS.

When querying an IP to find which AS (Autonomous System), it is helpful
to find associated IP addresses. The AS is part of the BGP routing for that IP.

A valid result returns an array of hash references.
The hash reference contains information about the AS such as the ASN,
the CIDR prefix of the AS, the Internet Registry (RIR) number (0 through 6),
the Description of the AS and the creation date for the AS.
An empty response returns an empty array ([]).
- **`cisco-umbrella-cli investigate get-domain-volume`** - List the query volume for a domain over the last 30 days.
If there is no information about the domain, Investigate returns an empty array.
As the query takes time to generate, the last two hours may be blank.
- **`cisco-umbrella-cli investigate get-domains-categorization`** - Look up the status, and security and content category IDs for the domain.
The domain status is a numerical value determined by the Cisco Security Labs team.
Valid status values are: '-1' (malicious), '1' (safe), or '0' (undetermined status).
- **`cisco-umbrella-cli investigate get-domains-risk-score`** - The Investigate Risk Score is based on an analysis of
the lexical characteristics of the domain name
and patterns in queries and requests to the domain.
The risk score is scaled from 0 to 100 where 100 is the highest risk
and 0 represents no risk at all. Periodically, Investigate updates this score based
on additional inputs.
A domain blocked by Umbrella receives a score of 100.
- **`cisco-umbrella-cli investigate get-links-name`** - List the domain names that are frequently requested around
the same time (up to 60 seconds before or after) as the given domain name,
but that are not frequently associated with other domain names.
- **`cisco-umbrella-cli investigate get-passive-dnsdomain`** - Get the Resource Record (RR) data for DNS responses, and categorization data, where the answer (or rdata) is the domain(s).
- **`cisco-umbrella-cli investigate get-passive-dnsip`** - Get the Resource Record (RR) data for DNS responses, and categorization data, where the answer (or rdata) is the domain(s).
- **`cisco-umbrella-cli investigate get-passive-dnsname`** - The Passive DNS endpoint provides historical data from the Umbrella resolvers for domains, IPs, and other resource records.
- **`cisco-umbrella-cli investigate get-passive-dnsraw`** - Get the Resource Record (RR) data for DNS responses, and categorization data, where the answer (or rdata) could be anything.
- **`cisco-umbrella-cli investigate get-recommendations-name`** - List the co-occurences for the specified domain.
A co-occurrence is when two or more domains are accessed by the same users within a small window of time.
Co-occurring domains are not necessarily problematic; legitimate sites co-occur with each other as a part
of normal web activity. However, unusual or suspicious co-occurences can provide additional information regarding attacks.
To determine co-occurrences for a domain, a small time window of traffic across all of our datacenters is taken.
Investigate checks the sites that end users visited before and after the domain was requested in the API call.
- **`cisco-umbrella-cli investigate get-sample-hash`** - Gather the information from the /samples endpoint, then pivot using the checksums
of the samples revealed in your initial query.
This pivot can reveal large chunks of new data about the malware being researched.
Returns a variety of data as nested JSON arrays.
The initial results array contains the information about the original sample.
These results are described first and are, in effect, the samples of the sample.
- **`cisco-umbrella-cli investigate get-sample-hash-artifacts`** - Other samples associated with this sample. The sample data does not include a threat score.
Artifacts are only available for Cisco Secure Malware Analytics customers.
- **`cisco-umbrella-cli investigate get-sample-hash-behaviors`** - Get the information about specific actions or unique properties of this sample,
especially local to your network or the computer where the sample is run.
- **`cisco-umbrella-cli investigate get-sample-hash-connections`** - Information about network activity associated with this sample, such as connections to other domains or IPs.
- **`cisco-umbrella-cli investigate get-samples-destination`** - Specify a domain, IP, or URL. Use the destination to search for all samples
associated with the destination. The default number of items in a response is 10. You can extend the limit.
You must have a license for Cisco Secure Malware Analytics to receive the samples data.

Cisco Secure Malware Analytics retains checksum samples for one year.
You may find that Investigate previously listed a sample related
to a destination. If Cisco Secure Malware Analytics no longer contains a sample
related to the destination, Investigate does not display the sample in the list
of associated samples.

An error may occur when the requested destination is not in a valid format,
if the requested host is not found in our database, or if there is no data available
for the destination that you have requested. CIDR subnets (for example: 10.10.10.0/24) and pattern search is not supported.
- **`cisco-umbrella-cli investigate get-search`** - List the newly seen domains that match a regular expression pattern.
- **`cisco-umbrella-cli investigate get-security-name`** - List multiple scores or security features for a domain. You can use the scores
or security features to determine relevant data points and build insights on the reputation
or security risk posed by the site. No one security information feature is conclusive.
Instead, consider these features as part of your security research.
- **`cisco-umbrella-cli investigate get-sub-domains`** - Get the subdomains of a given domain. If there is no subdomain for the domain, Investigate returns an empty array.
- **`cisco-umbrella-cli investigate get-timeline`** - List the historical tagging timeline for a given IP, domain, or URL.
Investigate sorts the timeline items in descending order using the timestamp field.
Each timeline item includes lists of security category,
attack, or threat type associated with the destination.
Use the Tagging Timeline endpoint to verify when Umbrella assigned or removed
a security category, attack, or threat type.
If the current timeline item contains the security category, type of attack,
or threat type not found in the previous timeline item,
Umbrella updated the current timeline item.
If the current timeline item does not contain the security category, attack,
or threat type found in the previous timeline item,
Umbrella removed the security category, type of attack,
or threat type.
- **`cisco-umbrella-cli investigate get-topmillion`** - List the most seen domains in Umbrella. You can download the data in a zip file,
or use the Investigate API to stream the data into a SIEM.
The popularity list contains our most queried domains based on passive DNS usage across
our Umbrella global network of more than 180 billion requests per day with many tens of millions
of unique active users, in more than 165 countries.
The metric does not only consist of browser-based http requests from users but also takes
in to account the number of unique client IPs invoking this domain relative to the sum of all requests
to all domains.
Our popularity ranking reflects the domain's relative internet activity agnostic
to the invocation protocols and applications where as site ranking models (such as Alexa) focus
on the web activity over port 80 (primarily from browsers).
In addition, the Umbrella popularity algorithm also applies data normalization techniques
to smooth potential biases that may occur due to sampling of DNS usage data.
- **`cisco-umbrella-cli investigate get-whois`** - Get the WHOIS information for the specified email addresses,
nameservers, and domains. You can search by multiple email addresses or multiple nameservers.
This documentation outlines the following API endpoints:
email (single and multiple), domain record (current and historical), and nameserver (single and multiple).
In some instances, WHOIS information can be irregular as there are no standards
between domain registrars and large volumes of information can be returned from a query.
As such, both the email and nameserver WHOIS endpoints have a limit of 500 results,
which you can reduce to a smaller set of results.
There is an `offset` parameter that can be leveraged to retrieve the entire set
of domain entries for a given email without any limitation. Only the email parameter
supports this.
You can sort the email parameter by filtering the entries based on the timestamp field.
If a domain, email, or nameserver has no known WHOIS information, Investigate returns `HTTP 404`.
If a domain, email or nameserver does not exist, Investigate returns `HTTP 404`.
- **`cisco-umbrella-cli investigate get-whois-email`** - Get the email address or addresses of the registrar for the domain or domains. The results include
the total number of results for domains registered by this email address and a list
of the first 500 domains associated with this email.
You can pivot on the email address to find other malicious domains registered by the same email.
This endpoint is limited to a maximum of 500 results, which are the first 500 gathered from the database.
Reduce the number of results by setting the `limit` query parameter.
**Note:** Due to the sample length, Investigate may truncate a sample.
- **`cisco-umbrella-cli investigate get-whois-history`** - Get a standard WHOIS response record for a single domain with available historical
WHOIS data returned in an object. The information displayed varies by registrant.
The default limit for history is 10. You can set another value with the `limit` query parameter.
- **`cisco-umbrella-cli investigate get-whois-nameserver`** - Get WHOIS information for the nameserver.
As a nameserver can potentially register hundreds or thousands of domains,
the server limits the number of results to 500.
- **`cisco-umbrella-cli investigate get-whois-nameservers`** - Get WHOIS information for the nameservers. To search by multiple nameservers, provide
a comma-delimited list of domain names for the `nameServerList` query parameter.
For example: `ns1.google.com,ns2.google.com`.
- **`cisco-umbrella-cli investigate get-whois-search`** - Performs a regular expression (RegEx) search on the WHOIS data (domain, nameserver, and email fields)
that was updated or created in the specified time range.
Returns a list of ten WHOIS records that match the specified RegEx expression.
Use the `offset` query parameter to paginate the collection.
By default, Investigate sorts by the `updated` field.
- **`cisco-umbrella-cli investigate post-domains-categorization`** - Provide a list of domains and look up the status, and security and content category IDs for each domain.
The domain status is a numerical value determined by the Cisco Security Labs team.
Valid status values are: '-1' (malicious), '1' (safe), or '0' (undetermined status).

### managed_customers

Manage managed customers

- **`cisco-umbrella-cli managed-customers create`** - Create a customer for a managed provider.
- **`cisco-umbrella-cli managed-customers delete`** - Delete an customer for the managed provider.
- **`cisco-umbrella-cli managed-customers get`** - Get a customer for the managed provider.
- **`cisco-umbrella-cli managed-customers list`** - List the customers for the managed provider.
- **`cisco-umbrella-cli managed-customers update`** - Update a customer for a managed provider.

### network_devices

Manage network devices

- **`cisco-umbrella-cli network-devices create`** - Create a network device.
- **`cisco-umbrella-cli network-devices delete`** - Remove a network device.
- **`cisco-umbrella-cli network-devices get`** - Get a network device.
- **`cisco-umbrella-cli network-devices list`** - List the network devices.
- **`cisco-umbrella-cli network-devices list-policies`** - List the Umbrella policies associated with a network device.
If no filters are supplied, Umbrella returns the DNS policies.
- **`cisco-umbrella-cli network-devices update`** - Update a network device.

### networks

Manage networks

- **`cisco-umbrella-cli networks create`** - Create a network. Before you can create a network, contact Support to get your IP range verified.
- **`cisco-umbrella-cli networks delete`** - Delete a network.
- **`cisco-umbrella-cli networks get`** - Get a Network.
- **`cisco-umbrella-cli networks list`** - List the Networks.
- **`cisco-umbrella-cli networks list-policies`** - List the policies for a network.
- **`cisco-umbrella-cli networks update`** - Update a Network. Before you can update the network's IP address, contact Support to get your IP range verified.

### organizations

Manage organizations

- **`cisco-umbrella-cli organizations`** - Get information about your provider organizations.
To query the collection, provide an email address for a member of an Umbrella provider organization.

### password_resets

Manage password resets

- **`cisco-umbrella-cli password-resets <customerId>`** - Renew the passwords for the user accounts in the child (customer) organization of a provider organization.

For each email address, Umbrella identifies the user's account and renews the password for the user account.
Umbrella does not send a renewal request to the user account's email address.

Once an admin resets the user account password through the `/passwordResets/{customerId}` API endpoint,
a user can sign into their account on the child (customer) organization's managed console
and reset the password for their user account.

The `/passwordResets/{customerId}` API endpoint is only available for parent (provider) organizations
on the Multi-org or provider console.

### policies

Manage policies

- **`cisco-umbrella-cli policies delete-policy-identities`** - Remove an identity from an Umbrella policy.
Policy changes may require up to 20 minutes to take effect globally.
For DNS policies, TTLs, caching, and session reuse may cause some devices and domains to appear to take longer to update.
- **`cisco-umbrella-cli policies list`** - List the Umbrella policies. You can filter by policy type.
If you do not specify a policy type, Umbrella returns the DNS policies.
- **`cisco-umbrella-cli policies update-policy-identities`** - Add an identity to your policy.
Policy changes may require up to 20 minutes to take effect globally.
For DNS policies, TTLs, caching, and session reuse may cause some devices
and domains to appear to take longer to update.

### provider_customers

Manage provider customers

- **`cisco-umbrella-cli provider-customers convert-trial-customer`** - Convert your Managed Services License Agreement (MSLA) trial to an MSLA customer.
- **`cisco-umbrella-cli provider-customers create`** - Create a customer for a provider.
- **`cisco-umbrella-cli provider-customers create-customer-access-request`** - Create the access request for the customer's organization.
- **`cisco-umbrella-cli provider-customers create-customer-trial-extensions`** - Create the extension for the customer's trial.
- **`cisco-umbrella-cli provider-customers delete`** - Delete a customer for a provider.
- **`cisco-umbrella-cli provider-customers get`** - Get a customer for the provider.
- **`cisco-umbrella-cli provider-customers get-customer-access-request`** - Get the access request details for the customer's organization.
- **`cisco-umbrella-cli provider-customers get-customer-addresses`** - Get the customer addresses for provider.
- **`cisco-umbrella-cli provider-customers get-customer-deals`** - Get the information about deals available to customer.
- **`cisco-umbrella-cli provider-customers get-customer-packages`** - Get the packages for the trial customer.
- **`cisco-umbrella-cli provider-customers get-customer-subscription-details`** - Get the subscription details for the customer's organization.
- **`cisco-umbrella-cli provider-customers get-customer-trial-strength`** - Get the strength of a customer trial for a provider.
- **`cisco-umbrella-cli provider-customers list`** - List the customers for the provider.
- **`cisco-umbrella-cli provider-customers update`** - Update a customer for a provider.
- **`cisco-umbrella-cli provider-customers update-customer-access-request`** - Update the access request for the customer's organization.
- **`cisco-umbrella-cli provider-customers update-customer-deals`** - Update Customer Deals by ID

### provider_reports

Manage provider reports

- **`cisco-umbrella-cli provider-reports create-customer-security-report-request`** - Create security report request for customer of service provider.
- **`cisco-umbrella-cli provider-reports get-deployment-status-by-organization`** - List the summary counts of deployment status for the organization within the timeframe.

**Access Scope:** Reports > Customer > Read-Only
- **`cisco-umbrella-cli provider-reports get-provider-categories`** - List the Provider categories

**Access Scope:** Reports > Customer > Read-Only
- **`cisco-umbrella-cli provider-reports get-provider-category-requests-by-organization`** - List the summary counts of all requests for each category within the timeframe.

**Access Scope:** Reports > Customer > Read-Only
- **`cisco-umbrella-cli provider-reports get-provider-requests-by-category`** - List the summary counts of all requests within the timeframe.

**Access Scope:** Reports > Customer > Read-Only
- **`cisco-umbrella-cli provider-reports get-provider-requests-by-destination`** - List the summary counts of all requests within the timeframe.

**Access Scope:** Reports > Customer > Read-Only
- **`cisco-umbrella-cli provider-reports get-provider-requests-by-hour`** - List the activity volume within the timeframe.

**Access Scope:** Reports > Customer > Read-Only
- **`cisco-umbrella-cli provider-reports get-provider-requests-by-organization`** - List the summary counts of all requests within the timeframe.

**Access Scope:** Reports > Customer > Read-Only
- **`cisco-umbrella-cli provider-reports get-provider-requests-by-timerange`** - List the activity volume within the timeframe.

**Access Scope:** Reports > Customer > Read-Only
- **`cisco-umbrella-cli provider-reports get-providers-console-data`** - Get the data available from the provider's console.
- **`cisco-umbrella-cli provider-reports get-service-provider-customer-security-report`** - Get the security report for the customer of the service provider.

### reports

Manage reports

- **`cisco-umbrella-cli reports get-applications`** - List the applications.

**Access Scope:** Reports > Utilities > Read-Only
- **`cisco-umbrella-cli reports get-bandwidth-by-hour`** - List the bandwidth in bytes within the timeframe. Only returns proxy data.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-bandwidth-by-timerange`** - List the bandwidth in bytes within the timeframe. Only returns proxy data.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-categories`** - List the categories.

**Access Scope:** Reports > Utilities > Read-Only
- **`cisco-umbrella-cli reports get-deployment-status`** - List the deployment status within the timeframe.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-identities`** - List the identities.

**Access Scope:** Reports > Utilities > Read-Only
- **`cisco-umbrella-cli reports get-identity`** - Get identity by identity ID.

**Access Scope:** Reports > Utilities > Read-Only
- **`cisco-umbrella-cli reports get-identity-distribution`** - List the number of requests by identity types.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-identity-distribution-type`** - List the number of requests by identity for the type of traffic.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-organization-requests-by-hour`** - List the activity volume within the timeframe.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-requests-by-hour`** - List the activity volume within the timeframe.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-requests-by-hour-and-category`** - List the activity volume within the timeframe by type of category.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-requests-by-hour-and-category-and-type`** - List the activity volume for the type of category within the timeframe.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-requests-by-timerange`** - List the activity volume within the timeframe.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-requests-by-timerange-and-category`** - List the activity volume within the timeframe by category.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-requests-by-timerange-and-category-type`** - List the activity volume within the timeframe by category.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-requests-by-timerange-type`** - List the activity volume within the timeframe.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-summaries-by-category`** - List the summaries of requests by category.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-summaries-by-category-type`** - List the summaries by category for the type of request.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-summaries-by-destination`** - List the summaries by destination.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-summaries-by-destination-type`** - List the summaries by destination for the type of traffic.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-summaries-by-rule-intrusion`** - List the summaries by rule for the intrusion type.

**Access Scope:** Reports > Summaries by rule > Read-Only
- **`cisco-umbrella-cli reports get-summary`** - Get the summary report.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-summary-by-type`** - Get the summary of requests by the traffic type.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-threat-name`** - Get the threat name by threat ID.

**Access Scope:** Reports > Utilities > Read-Only
- **`cisco-umbrella-cli reports get-threat-names`** - List the threat names.

**Access Scope:** Reports > Utilities > Read-Only
- **`cisco-umbrella-cli reports get-threat-type`** - List the threat types by threat ID.

**Access Scope:** Reports > Utilities > Read-Only
- **`cisco-umbrella-cli reports get-threat-types`** - List the threat types.

**Access Scope:** Reports > Utilities > Read-Only
- **`cisco-umbrella-cli reports get-top-categories`** - List the categories that received the greatest number of requests.
Order the number of requests in descending order.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-top-categories-type`** - List the categories for the type of traffic that received the greatest number of requests.
Order the number of requests in descending order.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-top-destinations`** - List the destinations by the number of requests made to this destination.
Return the results in descending order.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-top-destinations-type`** - List the destinations by type of destination and the number of requests made
to this destination. Return the collection in descending order.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-top-dnsquery-types`** - List the top types of DNS query.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-top-event-types`** - List the top event types by the number of requests made for each type of event.
Order the number of requests in descending order.
The valid event types are: `domain_security`, `domain_integration`,
`url_security`, `url_integration`, `cisco_amp`, and `antivirus`.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-top-files-proxy`** - List the top files within the timeframe. Only returns proxy data.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-top-identities`** - List the identities by the number of requests made, sorted in descending order.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-top-identities-type`** - List the identities for the specific traffic type by the number of requests.
Sort the results in descending order.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-top-ips`** - List the top IP addresses.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-top-ips-internal`** - List the top internal IP addresses.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-top-threat-types`** - List the top types of threat within the timeframe. Returns both DNS and proxy data.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-top-threat-types-by-type`** - List the top threat-types within the timeframe.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-top-threats`** - Get the top threats within the timeframe. Returns both DNS and proxy data.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-top-threats-by-type`** - Get the top threats within the timeframe.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-top-urls`** - List the top number of URLs that are requested for a certain domain.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-total-requests`** - Get the count of the total requests.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports get-total-requests-by-type`** - Get the count of the total requests for the request type.

**Access Scope:** Reports > Aggregations > Read-Only
- **`cisco-umbrella-cli reports post-identities`** - Get the identities information by providing a list of identity IDs in the request body.

**Access Scope:** Reports > Utilities > Read-Only

### roaming_computers

Manage roaming computers

- **`cisco-umbrella-cli roaming-computers delete`** - Delete a roaming computer in the organization.
- **`cisco-umbrella-cli roaming-computers get`** - Get a roaming computer in the organization.
- **`cisco-umbrella-cli roaming-computers get-organization-info`** - Get the OrgInfo.json properties for deploying the Cisco Secure Client on user devices in the organization.
The Cisco Secure Client with the Internet Security module requires the OrgInfo.json properties.
- **`cisco-umbrella-cli roaming-computers list`** - List the roaming computers.
- **`cisco-umbrella-cli roaming-computers update`** - Update a roaming computer in the organization.

### roles

Manage roles

- **`cisco-umbrella-cli roles`** - List the roles in the organization.

### s3_bucket_keys

Manage s3 bucket keys

- **`cisco-umbrella-cli s3-bucket-keys`** - Rotate the Cisco-managed S3 bucket key for the organization. Refused here: Umbrella shows the new secret only once and this program withholds it, so do this in the Umbrella dashboard.

### sites

Manage sites

- **`cisco-umbrella-cli sites create`** - Create a site.
- **`cisco-umbrella-cli sites delete`** - Delete a site.
- **`cisco-umbrella-cli sites get`** - Get a site in the organization.
- **`cisco-umbrella-cli sites list`** - List the sites in the organization.
- **`cisco-umbrella-cli sites update`** - Update a site.

### swg_device_settings

Manage swg device settings

- **`cisco-umbrella-cli swg-device-settings list`** - List the Secure Web Gateway (SWG) override setting for devices in the organization.
- **`cisco-umbrella-cli swg-device-settings remove`** - Remove the Secure Web Gateway (SWG) override setting for the devices in the organization.
Once you remove the override setting on a device, Umbrella applies your organization's SWG setting to the device.
- **`cisco-umbrella-cli swg-device-settings set`** - Add a list of origin ID and the Secure Web Gateway setting for devices in the organization.
The SWG device setting overrides the organization-level SWG setting.

**Note:** Before you can add an SWG override setting to a device, you must register the device as a roaming computer with Umbrella.
Umbrella applies the SWG override setting to a device using the device's origin ID.
You can list the roaming computers in your organization by sending a request to the Umbrella Roaming Computers API.

### tags

Manage tags

- **`cisco-umbrella-cli tags add-to-devices`** - For the tag ID, add the tag to the roaming computers.
- **`cisco-umbrella-cli tags create`** - Add a tag to the organization.
- **`cisco-umbrella-cli tags list`** - List the tags in the organization.
- **`cisco-umbrella-cli tags list-devices`** - List the roaming computers that have a tag with the specified tag ID.
- **`cisco-umbrella-cli tags remove-from-devices`** - For the tag ID, remove the tag on the roaming computers in the organization.
After the delete, if the tag is not assigned to any roaming computers in the organization, Umbrella removes the tag from
the organization. You can recreate the tag for your organization.

### tunnels

Manage tunnels

- **`cisco-umbrella-cli tunnels add`** - Add a new tunnel to the organization. Refused here unless you choose the secret yourself: Umbrella shows a generated one only once and this program withholds it, so otherwise do this in the Umbrella dashboard.
- **`cisco-umbrella-cli tunnels delete`** - Delete a tunnel in the organization.
- **`cisco-umbrella-cli tunnels get`** - Get a specific tunnel.
- **`cisco-umbrella-cli tunnels get-error-events`** - Get the recent tunnel error events.
- **`cisco-umbrella-cli tunnels get-global-error-events`** - Get the recent global error events.
- **`cisco-umbrella-cli tunnels get-org-state`** - List the tunnel state information.
- **`cisco-umbrella-cli tunnels get-policies`** - List the policies that include the network tunnel.
- **`cisco-umbrella-cli tunnels get-state`** - Get the tunnel state information.
- **`cisco-umbrella-cli tunnels list`** - List the tunnels for an organization.
- **`cisco-umbrella-cli tunnels update`** - Update the `name`, `siteOriginId`, `networkCIDRs`, and client `deviceType` properties for a tunnel.
Updates to read-only attributes are ignored.
- **`cisco-umbrella-cli tunnels update-credentials`** - Update and rotate the tunnel credentials. Refused here unless you choose the secret yourself: Umbrella shows a generated one only once and this program withholds it, so otherwise do this in the Umbrella dashboard.

### users

Manage users

- **`cisco-umbrella-cli users create`** - Create an Umbrella user account with a designated role.

Once a user account is deleted, you can recreate the account through the `POST` operation.
When you recreate the user account, only set the `email` and `roleId` fields in the Request Body.

For example: {
  "email": "DEVWKS-22@mailinator.com",
  "roleId": 1
}

If you provide all fields for the user account in the Request Body, Umbrella returns an `HTTP/400` (Bad Request) with the
message: `Email already in use`.
- **`cisco-umbrella-cli users delete`** - Delete a user account.
- **`cisco-umbrella-cli users get`** - Get a user account.
- **`cisco-umbrella-cli users list`** - List the user accounts in the organization.

### virtual_appliances

Manage virtual appliances

- **`cisco-umbrella-cli virtual-appliances delete`** - Delete a virtual appliance in the organization.
- **`cisco-umbrella-cli virtual-appliances get`** - Get a virtual appliance in the organization.
- **`cisco-umbrella-cli virtual-appliances list`** - List the virtual appliances in the organization.
- **`cisco-umbrella-cli virtual-appliances update`** - Update a virtual appliance in the organization.


### allowlist


- **`cisco-umbrella-cli allowlist audit`** - Find allow-list entries that are now categorized as malicious, entries that conflict with a block list, and allows added in a recent window.


### destination-lists


- **`cisco-umbrella-cli destination-lists diff`** - Reconcile a destination list against a source-of-truth file: see what to add and remove, then apply it in API-sized chunks. --apply also removes entries absent from the file unless --no-remove.


### fleet


- **`cisco-umbrella-cli fleet health`** - One row per managed customer: seats, an issues count, and counts of off, uninstalled, stale, and SWG-unprotected roaming computers, unhealthy virtual appliances, and down tunnels. Customers that fail are listed under fetch_failures.


### hygiene


- **`cisco-umbrella-cli hygiene`** - Device-level findings in one org: stale or offline roaming computers, unhealthy virtual appliances, down tunnels, and unverified networks, each tagged with a kind.


### report-card


- **`cisco-umbrella-cli report-card`** - A customer's security summary, top threat types, blocked categories, and deployment status for a window, compared with the prior window. --csv emits the summary rows only.


### verdict


- **`cisco-umbrella-cli verdict`** - One answer for a domain: category, risk score, security status, every destination list that blocks or allows it (including parent-domain entries), and who resolved it recently.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`cisco-umbrella-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`cisco-umbrella-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`cisco-umbrella-cli learnings list`** - Inspect taught rows
- **`cisco-umbrella-cli learnings forget <query>`** - Undo a teach
- **`cisco-umbrella-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`cisco-umbrella-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`cisco-umbrella-cli teach-pattern`** - Install a query/resource template up front
- **`cisco-umbrella-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `CISCO_UMBRELLA_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `cisco-umbrella-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
cisco-umbrella-cli api-keys list

# JSON for scripting and agents
cisco-umbrella-cli api-keys list --json
# Filter to specific fields
cisco-umbrella-cli api-keys list --json --select keys,limit,message

# Dry run  -  show the request without sending
cisco-umbrella-cli api-keys list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
cisco-umbrella-cli api-keys list --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending; `auth login`, `auth set-token` and `auth logout` honour it too, and `export --dry-run` leaves its output file alone
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **Explicit confirmation** - `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

## Health Check

```bash
cisco-umbrella-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `cisco-umbrella-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/cisco-umbrella-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\cisco-umbrella-cli\config.toml` on Windows; `--home`, `CISCO_UMBRELLA_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `CISCO_UMBRELLA_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `CISCO_UMBRELLA_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `CISCO_UMBRELLA_API_KEY` | per_call | Yes | Umbrella API Key (OAuth client ID). |
| `CISCO_UMBRELLA_API_SECRET` | per_call | Yes | Umbrella Key Secret (OAuth client secret). |
| `CISCO_UMBRELLA_ORG_ID` | per_call | No | Child org ID to act on from an MSP or Multi-org parent key (same as `--org`). |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `cisco-umbrella-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `cisco-umbrella-cli doctor` to check credentials
- Verify both variables are set: `echo $CISCO_UMBRELLA_API_KEY $CISCO_UMBRELLA_API_SECRET | wc -w` prints 2
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **HTTP 403 Access Forbidden on one command family**  -  The API key lacks that scope; edit the key in Admin > API Keys and add the scope (deployments, policies, reports, investigate, or admin).
- **HTTP 429 Too Many Requests during sync**  -  Deployments and admin allow 14 requests per minute per key; rerun with --rate-limit 0.2 or sync fewer resources with --resources.
- **Results show the parent console instead of a customer**  -  Set CISCO_UMBRELLA_ORG_ID to the child org ID (from managed-customers list) or pass --org.
- **Reports return 400 for missing from/to**  -  Reporting needs a window, for example --from -1days --to now.
- **fleet health: no managed or provider customers visible to this key**  -  Use an API key created in the MSP, Multi-org, or provider console (Console Settings > API Keys) with the admin customers scope, and drop --org.
- **hygiene returns no findings with a 'nothing synced' note**  -  Run cisco-umbrella-cli sync --resources roaming_computers,virtual_appliances,tunnels-tunnels-state,networks (add the same --org) first.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**Azure Sentinel Cisco Umbrella playbooks**](https://github.com/Azure/Azure-Sentinel/tree/master/Solutions/CiscoUmbrella)  -  Python (6189 stars)
- [**Cortex XSOAR Cisco Umbrella packs**](https://github.com/demisto/content/tree/master/Packs/Cisco-umbrella-cloud-security)  -  Python (1313 stars)
- [**TheHive Cortex Umbrella analyzer**](https://github.com/TheHive-Project/Cortex-Analyzers/tree/master/analyzers/CiscoUmbrella)  -  Python (493 stars)
- [**Cisco DevNet cloud-security samples + Postman**](https://github.com/CiscoDevNet/cloud-security)  -  Python (65 stars)
- [**umbr_api**](https://github.com/agrechin/umbr_api)  -  Python (9 stars)
- [**MSPbots cisco-umbrella-mcp**](https://github.com/MSPbotsAI/cisco-umbrella-mcp)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
