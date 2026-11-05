# Blackpoint Cyber CompassOne CLI

**CompassOne from the terminal: tenants, detections, assets, vulnerabilities, posture, reports and Cloud Response admin, plus a cross-tenant fleet view of open detections, stale agents and coverage gaps.**

Commands for the CompassOne endpoints documented in Blackpoint's public SDK and the community MCP servers, with --dry-run on every write. fleet-sync pulls every tenant into a local store so triage, gaps, stale-agents, cve-spread and leaderboard answer cross-customer questions in one command, offline.

Learn more at [Blackpoint Cyber CompassOne](https://blackpointcyber.com).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `blackpoint-cli` and `blackpoint-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blackpoint/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blackpoint/install.ps1 | iex
   ```
3. Verify: `blackpoint-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=blackpoint). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/blackpoint --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/blackpoint --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `blackpoint-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the blackpoint skill from https://github.com/Servosity/msp-skills/tree/main/skills/blackpoint. The skill defines how its required CLI (`blackpoint-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=blackpoint).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `BLACKPOINT_API_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blackpoint/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blackpoint/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "blackpoint": {
      "command": "blackpoint-mcp",
      "env": {
        "BLACKPOINT_API_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`blackpoint-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `BLACKPOINT_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `BLACKPOINT_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export BLACKPOINT_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
blackpoint-mcp --transport http
```

Set `BLACKPOINT_MCP_READ_ONLY=1` (exactly `1`) to list only read-only MCP tools: it removes `blackpoint_execute`, `sync`, `fleet_sync`, `import`, `export`, `workflow_archive` and the learn-store writers (`teach`, `teach_lookup`, `teach_pattern`, `teach_playbook`, `playbook_amend`, `learnings_confirm`, `learnings_reject`, `learnings_forget`). Live reads stay available through `blackpoint_get` and `blackpoint_read`, and `blackpoint_read` refuses every state-changing endpoint.

## Authentication

Create an API token in the CompassOne portal and export BLACKPOINT_API_TOKEN. A partner token sees every tenant; a tenant token sees one customer. Tenant-scoped commands (detections, assets, vulnerabilities, scans, posture, reports) take --x-tenant-id <tenant uuid>, which you get from 'tenants list'. Cloud Response commands take --tenant-id instead.

## Quick Start

```bash
# Confirm the CLI is installed (dry run, no network); run 'blackpoint-cli doctor' for the real auth check.
blackpoint-cli doctor --dry-run

# Find the tenant ids every scoped command needs; type SELF is your own tenant.
blackpoint-cli tenants list --agent --select id,name,type

# Open detections for one customer.
blackpoint-cli detections list --x-tenant-id 3f2a9c1e-7b4d-4e8a-9f10-2c6d8b5e4a71 --status OPEN --agent

# Pull every tenant into the local store; the cross-tenant commands read only this store.
blackpoint-cli fleet-sync

# The cross-tenant open-detection queue, oldest first.
blackpoint-cli triage --older-than 24h

```

## Unique Features

### Cross-tenant fleet view
- **`triage`**  -  One oldest-first queue of open detections across every customer tenant, with tenant name, age, alert count and ticket status. Reads the local store, so run fleet-sync first; before that it returns an empty list with a stderr hint.

  _Reach for this for the morning SOC sweep instead of calling detections list once per tenant._

  ```bash
  blackpoint-cli triage --older-than 24h --type MDR --agent
  ```
- **`gaps`**  -  Find tenants with no recent completed scan, no or empty escalation contact group, or no posture rating; scans_unknown, contacts_unknown or posture_unknown means that data could not be fetched. Reads the local store, so run fleet-sync first; before that it returns an empty list with a stderr hint.

  _Use it to answer which customers are unprotected or unreachable before an incident proves it._

  ```bash
  blackpoint-cli gaps --scan-days 30 --agent
  ```
- **`stale-agents`**  -  List devices across every tenant whose agent has not reported in N days or never checked in, with Defender status; decommissioned devices are skipped unless --include-decommissioned. Reads the local store, so run fleet-sync first; before that it returns an empty list with a stderr hint.

  _Use it for weekly endpoint hygiene across the whole book of customers._

  ```bash
  blackpoint-cli stale-agents --days 14 --agent
  ```
- **`cve-spread`**  -  Rank unresolved CVEs by how many tenants and assets they affect; --exploited or --prioritized narrows the list. Needs the vulnerability-management entitlement: when every tenant's fetch was refused (403) it fails instead of returning an empty list. Reads the local store, so run fleet-sync first; before that it returns an empty list with a stderr hint.

  _Use it to decide which patch campaign protects the most customers first._

  ```bash
  blackpoint-cli cve-spread --exploited --agent
  ```
- **`fleet-sync`**  -  Pull every tenant's detections, devices, vulnerabilities, scans, exposures, posture, contact groups and approved countries into the local store; failed or incomplete fetches are kept, reported under fetch_failures, and flagged by the readers.

  _Run it before triage, gaps, stale-agents, cve-spread, overview, leaderboard or geo-audit so they read fresh data._

  ```bash
  blackpoint-cli fleet-sync --agent
  ```

### Client reporting
- **`client-report`**  -  Live: build one client's reporting-window summary from six CompassOne endpoints (posture, detection count and weekly trend, top entities and threats, latest report run per type). Needs BLACKPOINT_API_TOKEN; failed sections are listed under fetch_failures.

  _Use it when an agent drafts a monthly client security report; one call replaces six._

  ```bash
  blackpoint-cli client-report "Acme Dental" --since 30d --agent
  ```
- **`overview`**  -  One offline current-state view of a tenant: open detections, stale devices, exploited or prioritized CVEs, last scans, posture, dark-web exposure counts, cloud connection state and escalation contacts. Reads the local store, so run fleet-sync first; an unknown tenant or a store with no fleet data exits 3 (not found).

  _Use it before a client call or escalation to see everything about one customer at once._

  ```bash
  blackpoint-cli overview "Acme Dental" --agent
  ```
- **`leaderboard`**  -  Rank tenants by security posture score as a share of their maximum, with maturity level and weakest NIST function. Reads the local store, so run fleet-sync first; before that it returns an empty list with a stderr hint.

  _Use it to pick which clients need attention this week._

  ```bash
  blackpoint-cli leaderboard --agent
  ```

### Cloud Response admin
- **`geo-audit`**  -  Audit Cloud Response approved countries per connection and per user; flag expired or expiring travel exceptions and broad allow-lists. Reads the local store, so run fleet-sync first; before that it returns an empty list with a stderr hint.

  _Use it weekly so temporary travel exceptions do not silently become permanent._

  ```bash
  blackpoint-cli geo-audit --expiring 7d --agent
  ```

## Recipes

### Morning sweep

```bash
blackpoint-cli triage --older-than 24h --agent
```

Run fleet-sync first; then this lists open detections across all tenants, oldest first.

### Narrow a big detection payload

```bash
blackpoint-cli detections list --x-tenant-id 3f2a9c1e-7b4d-4e8a-9f10-2c6d8b5e4a71 --agent --select id,hostname,alertCount,ticket.status
```

Keep only the fields an agent needs from a nested alert-group page.

### Monthly client report

```bash
blackpoint-cli client-report "Acme Dental" --since 30d --agent
```

Live (needs BLACKPOINT_API_TOKEN): posture, detection counts, weekly trend, top entities and threats, and the latest report run per report type in the window.

### Who is uncovered

```bash
blackpoint-cli gaps --scan-days 30 --agent
```

Run fleet-sync first. Gap codes: no_recent_scan, no_contact_group, empty_contact_group, no_posture_rating; scans_unknown, contacts_unknown or posture_unknown means that data could not be fetched, not that coverage is missing.

## Usage

Run `blackpoint-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `BLACKPOINT_CONFIG_DIR`, `BLACKPOINT_DATA_DIR`, `BLACKPOINT_STATE_DIR`, or `BLACKPOINT_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `BLACKPOINT_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export BLACKPOINT_HOME=/srv/blackpoint
blackpoint-cli doctor
```

On Windows (PowerShell):

```powershell
$env:BLACKPOINT_HOME = "C:\ProgramData\blackpoint"
blackpoint-cli doctor
```

Under `BLACKPOINT_HOME=/srv/blackpoint`, the four dirs resolve to `/srv/blackpoint/config`, `/srv/blackpoint/data`, `/srv/blackpoint/state`, and `/srv/blackpoint/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:BLACKPOINT_HOME = "C:\srv\blackpoint"
blackpoint-cli doctor
```

Under `$env:BLACKPOINT_HOME = "C:\srv\blackpoint"`, the four dirs resolve to `C:\srv\blackpoint\config`, `C:\srv\blackpoint\data`, `C:\srv\blackpoint\state`, and `C:\srv\blackpoint\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "blackpoint": {
      "command": "blackpoint-mcp",
      "env": {
        "BLACKPOINT_HOME": "/srv/blackpoint"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `BLACKPOINT_DATA_DIR` overrides an explicit `--home` for that kind. Use `BLACKPOINT_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `BLACKPOINT_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `blackpoint-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read that is not scoped to one tenant is also written to the SQLite store (a read with `--x-tenant-id` or `tenantId` always goes live and is not saved) (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are cached for five minutes under `http/` in the cache directory. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `blackpoint-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `blackpoint-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `BLACKPOINT_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `BLACKPOINT_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### accounts

Partner accounts this token can see.

- **`blackpoint-cli accounts get`** - Returns one partner account.
- **`blackpoint-cli accounts list`** - Lists the partner accounts this token can see. Wire: GET /v1/accounts, returns {data, meta}.

### assets

Per-tenant asset inventory by class, and the relationship graph.

- **`blackpoint-cli assets get`** - Returns one asset; fields depend on its class.
- **`blackpoint-cli assets list`** - Lists assets for one tenant. `class` is required: there is no all-assets query. Classes populated today: DEVICE, USER, SOFTWARE, SOURCE, SURVEY. Requires x-tenant-id (400 without it).
- **`blackpoint-cli assets list-relationships`** - Lists what one asset is connected to in one direction. Use the finding classes (ALERT, ALERTGROUP, INCIDENT, VULNERABILITY) for what has been found on a machine. The far-side class is sent as `class`; some clients send it as `entityClass`.

### cisco_onboardings

Cisco Duo Cloud Response onboarding.

- **`blackpoint-cli cisco-onboardings complete`** - Completes the onboarding by verifying everything was set up appropriately.
- **`blackpoint-cli cisco-onboardings create`** - Begins a new onboarding for a customer. Fails if the onboarding for that domain already exists in Blackpoint. Body: Duo Admin API host, integration key and secret key (CCCiscoDuoOnboardingRequestDto). The secret key is sensitive.
- **`blackpoint-cli cisco-onboardings delete`** - Abandons an onboarding, removing the db records and cisco duo onboarding app if there is any.
- **`blackpoint-cli cisco-onboardings get`** - Get the state of an onboarding.
- **`blackpoint-cli cisco-onboardings list`** - List the states of the onboardings a customer has completed or in progress.
- **`blackpoint-cli cisco-onboardings sync-users`** - Runs the Duo post-provision step: re-verifies the onboarding and starts the user sync if it has not started or has stalled.

### cloud_connections

Cloud Response connection approved countries and per-user travel exceptions.

- **`blackpoint-cli cloud-connections approve-country`** - Approve a country for a given connection. Body: {isoCountryCode}.
- **`blackpoint-cli cloud-connections approve-user-country`** - Add approved countries for connection user. Body: {isoCountryCode, startDate?, endDate?} (a travel exception window).
- **`blackpoint-cli cloud-connections list-approved-countries`** - Get a paginated list of approved countries for a given connection.
- **`blackpoint-cli cloud-connections list-user-countries`** - List a user's approved countries
- **`blackpoint-cli cloud-connections list-users`** - Get users for connection.
- **`blackpoint-cli cloud-connections remove-approved-country`** - Remove a country for a given connection by its ID.
- **`blackpoint-cli cloud-connections remove-user-country`** - Delete connection user approved country.

### contact_groups

Escalation contact groups, their members and tenant assignment.

- **`blackpoint-cli contact-groups add-member`** - Add member to contact group.
- **`blackpoint-cli contact-groups assign-tenants`** - Assign tenants to a contact group
- **`blackpoint-cli contact-groups create`** - Body: TEV1CreateContactGroupRequestDto.
- **`blackpoint-cli contact-groups delete`** - Delete contact group.
- **`blackpoint-cli contact-groups delete-many`** - Bulk delete: DELETE with a JSON body {contactGroupIds: [...]}.
- **`blackpoint-cli contact-groups delete-member`** - Delete contact group member.
- **`blackpoint-cli contact-groups get`** - Get contact group.
- **`blackpoint-cli contact-groups get-member`** - Get contact group member.
- **`blackpoint-cli contact-groups list`** - Escalation contact groups for an account.
- **`blackpoint-cli contact-groups list-members`** - List contact group members.
- **`blackpoint-cli contact-groups list-tenants`** - List tenants assigned to a contact group.
- **`blackpoint-cli contact-groups list-unassigned-tenants`** - Get a paginated list of all tenants which have not being assigned to a given contact group.
- **`blackpoint-cli contact-groups update`** - Body: TEV1UpdateContactGroupRequestDto; members carry their id to keep them.
- **`blackpoint-cli contact-groups update-member`** - Body: TEV1UpdateContactGroupMemberRequestDto (all fields required).

### countries

ISO country reference list for Cloud Response geo policy.

- **`blackpoint-cli countries get`** - Get country by code.
- **`blackpoint-cli countries list`** - List countries. Reference list used by the approved-country (geo) policy.

### darkweb_exposures

Dark-web credential exposures from a tenant's latest scan.

- **`blackpoint-cli darkweb-exposures`** - Credential exposures from the tenant's most recent dark-web scan. Empty means the last scan found nothing; check `scans --type darkweb` for whether a scan ran.

### detections

Detections (alert groups) per tenant, with counts, trends and top entities and threats.

- **`blackpoint-cli detections count`** - Counts detections without listing them. Response shape is not published; returned verbatim.
- **`blackpoint-cli detections get`** - Returns one detection in full. Its nested ticket carries the lifecycle status: NEW, CLAIM, INVESTIGATE, ESCALATE, RESOLVE, CLOSE, INFORMATIONAL, NO_AGENT or SUGGEST_SUPPRESSION.
- **`blackpoint-cli detections list`** - Lists detections for one tenant, paged with skip/take. Requires the x-tenant-id header (400 `x-tenant-id header required` without it). The deprecated `tenantId` query parameter is not exposed. `status` is group-level OPEN/RESOLVED; the nine-state lifecycle lives on the nested ticket.
- **`blackpoint-cli detections list-alerts`** - Lists the individual alerts a detection aggregated.
- **`blackpoint-cli detections list-by-week`** - Detection counts bucketed by week (the trend series behind a monthly report chart). Response returned verbatim.
- **`blackpoint-cli detections list-top-by-entity`** - Ranks the hosts and users with the most detections. Response returned verbatim.
- **`blackpoint-cli detections list-top-by-threat`** - Ranks the threat types behind the most detections. Response returned verbatim.

### external_exposures

Findings from one external (perimeter) scan.

- **`blackpoint-cli external-exposures <scanId>`** - Findings from one external (perimeter) scan. Pick the scan with `scans --type external`. Response returned verbatim.

### google_onboardings

Google Workspace Cloud Response onboarding state.

- **`blackpoint-cli google-onboardings get`** - Get the state of an onboarding.
- **`blackpoint-cli google-onboardings list`** - List the states of the onboardings a customer has completed or in progress.

### ms365

Microsoft 365 Cloud Response connections, users and approved countries.

- **`blackpoint-cli ms365 approve-country`** - Post iso country to ms365 connection authorized country list. Body: {code}.
- **`blackpoint-cli ms365 approve-user-country`** - Adds an authorized country to user. Body: {isoCountryCode, startDate?, endDate?}.
- **`blackpoint-cli ms365 get-connection`** - Get cloud ms365 connections by ID. Includes onboarding stage, policy config, CIS benchmarks and events.
- **`blackpoint-cli ms365 list-approved-countries`** - List m365 connection approved countries. Returns the connection with authorizedCountries.
- **`blackpoint-cli ms365 list-connections`** - Get cloud ms365 customer. Returns the tenant with its ms365DefensePackages (one per connected Microsoft 365 tenant).
- **`blackpoint-cli ms365 list-user-countries`** - Get active isoCountries for a given user.
- **`blackpoint-cli ms365 list-users`** - Get ms ms365 defense connection's users.
- **`blackpoint-cli ms365 remove-country`** - Delete iso country from ms365 connection authorized country list. The country is a `code` query parameter.
- **`blackpoint-cli ms365 remove-user-country`** - Deletes an authorized country from user.

### posture

Security posture rating and metric breakdown for one tenant.

- **`blackpoint-cli posture`** - Score, maximumScore, maturityLevel and every metric row. maximumScore varies by edition (50 and 120 observed); a row with metricApplied false deducts nothing.

### reports

Generated report runs and their JSON payloads.

- **`blackpoint-cli reports get-json`** - The report payload, verbatim. `report` is polymorphic by reportType; the legacy Executive payload keys executiveSummary alongside the current securityPosture.
- **`blackpoint-cli reports list`** - Generated report runs for one tenant. startDate/endDate filter on intervalStart (inclusive).

### scans

Vulnerability scan runs for one tenant.

- **`blackpoint-cli scans`** - Scan runs and their status. A tenant with no recent completed scan has no coverage, not a clean bill of health.

### tenants

Customer tenants; the source of every tenant id.

- **`blackpoint-cli tenants get`** - Returns one tenant with its status, contact group (and members) and report/alert email lists.
- **`blackpoint-cli tenants list`** - Lists the customer tenants this token can reach. It takes no x-tenant-id header; it is what produces the tenant ids the scoped commands need. `search` matches both tenant id and name. Check `type` (SELF is the MSP's own tenant) before running customer reports.

### users

Portal users visible to this token.

- **`blackpoint-cli users`** - Portal users visible to this token. Response returned verbatim; 403 means the key is not entitled.

### vulnerabilities

Vulnerability findings for one tenant.

- **`blackpoint-cli vulnerabilities`** - CVE-level findings for one tenant with severity, scores and affected-asset counts. 403 when the account is not entitled to vulnerability management; tenant scoping via x-tenant-id has not been verified against a live tenant.


### client-report


- **`blackpoint-cli client-report`** - Live: build one client's reporting-window summary from six CompassOne endpoints (posture, detection count and weekly trend, top entities and threats, latest report run per type). Needs BLACKPOINT_API_TOKEN; failed sections are listed under fetch_failures.


### cve-spread


- **`blackpoint-cli cve-spread`** - Rank unresolved CVEs by how many tenants and assets they affect; --exploited or --prioritized narrows the list. Needs the vulnerability-management entitlement: when every tenant's fetch was refused (403) it fails instead of returning an empty list. Reads the local store, so run fleet-sync first; before that it returns an empty list with a stderr hint.


### fleet-sync


- **`blackpoint-cli fleet-sync`** - Pull every tenant's detections, devices, vulnerabilities, scans, exposures, posture, contact groups and approved countries into the local store; failed or incomplete fetches are kept, reported under fetch_failures, and flagged by the readers.


### gaps


- **`blackpoint-cli gaps`** - Find tenants with no recent completed scan, no or empty escalation contact group, or no posture rating; scans_unknown, contacts_unknown or posture_unknown means that data could not be fetched. Reads the local store, so run fleet-sync first; before that it returns an empty list with a stderr hint.


### geo-audit


- **`blackpoint-cli geo-audit`** - Audit Cloud Response approved countries per connection and per user; flag expired or expiring travel exceptions and broad allow-lists. Reads the local store, so run fleet-sync first; before that it returns an empty list with a stderr hint.


### leaderboard


- **`blackpoint-cli leaderboard`** - Rank tenants by security posture score as a share of their maximum, with maturity level and weakest NIST function. Reads the local store, so run fleet-sync first; before that it returns an empty list with a stderr hint.


### overview


- **`blackpoint-cli overview`** - One offline current-state view of a tenant: open detections, stale devices, exploited or prioritized CVEs, last scans, posture, dark-web exposure counts, cloud connection state and escalation contacts. Reads the local store, so run fleet-sync first; an unknown tenant or a store with no fleet data exits 3 (not found).


### stale-agents


- **`blackpoint-cli stale-agents`** - List devices across every tenant whose agent has not reported in N days or never checked in, with Defender status; decommissioned devices are skipped unless --include-decommissioned. Reads the local store, so run fleet-sync first; before that it returns an empty list with a stderr hint.


### triage


- **`blackpoint-cli triage`** - One oldest-first queue of open detections across every customer tenant, with tenant name, age, alert count and ticket status. Reads the local store, so run fleet-sync first; before that it returns an empty list with a stderr hint.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`blackpoint-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`blackpoint-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`blackpoint-cli learnings list`** - Inspect taught rows
- **`blackpoint-cli learnings forget <query>`** - Undo a teach
- **`blackpoint-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`blackpoint-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`blackpoint-cli teach-pattern`** - Install a query/resource template up front
- **`blackpoint-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `BLACKPOINT_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `blackpoint-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
blackpoint-cli accounts list

# JSON for scripting and agents
blackpoint-cli accounts list --json
# Filter to specific fields
blackpoint-cli accounts list --json --select billingVersion,created,id

# Dry run  -  show the request without sending
blackpoint-cli accounts list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
blackpoint-cli accounts list --agent
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
- **Piped input** - write commands can accept structured input when their help lists `--stdin`. The body is held as a credential: a `--dry-run` preview of it, and a vendor answer that repeats part of it, are withheld with a notice, and the write still happens. To see the answer, set the fields with flags
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error or not entitled (401/403), `1` other error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

## Health Check

```bash
blackpoint-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `blackpoint-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/blackpoint-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\blackpoint-cli\config.toml` on Windows; `--home`, `BLACKPOINT_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `BLACKPOINT_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `BLACKPOINT_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `BLACKPOINT_API_TOKEN` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `blackpoint-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `blackpoint-cli doctor` to check credentials
- Verify the environment variable is set: `echo $BLACKPOINT_API_TOKEN`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **400 x-tenant-id header required**  -  Pass --x-tenant-id with a tenant id from 'blackpoint-cli tenants list'.
- **401 No token provided**  -  export BLACKPOINT_API_TOKEN=<token from the CompassOne portal>
- **403 on vulnerabilities, scans or users**  -  The token's account is not entitled to that product or tenant; use a partner token with the product enabled.
- **429 Too Many Requests during fleet-sync**  -  The key quota is 2000 requests per 15 minutes; rerun fleet-sync with --tenant to limit the fan-out.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**MSPbotsAI/compassone-mcp**](https://github.com/MSPbotsAI/compassone-mcp)  -  Python
- [**WYRE-AI/blackpoint-mcp**](https://github.com/WYRE-AI/blackpoint-mcp)  -  TypeScript
- [**WYRE-AI/node-blackpoint**](https://github.com/WYRE-AI/node-blackpoint)  -  TypeScript
- [**derpenstiltskin/immybot-bpc-compassone**](https://github.com/derpenstiltskin/immybot-bpc-compassone)  -  PowerShell

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
