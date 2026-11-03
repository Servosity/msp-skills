---
name: crowdstrike
description: "Every CrowdStrike Falcon MSP operation, plus a Flight-Control-aware local store that answers fleet-wide questions across all your tenants at once  -  something no other Falcon tool (including the official MCP server) does. Trigger phrases: `check crowdstrike alerts across all tenants`, `show stale falcon sensors`, `critical vulnerabilities across my crowdstrike fleet`, `crowdstrike tenant scorecard`, `list falcon child CIDs`, `use crowdstrike-cli`, `run crowdstrike-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "CrowdStrike"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - crowdstrike-cli
    install:
      - kind: script
        bins: [crowdstrike-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/crowdstrike/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/crowdstrike/install.ps1
---

# CrowdStrike Falcon - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `crowdstrike-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/crowdstrike/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/crowdstrike/install.ps1 | iex
   ```
3. Verify: `crowdstrike-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `crowdstrike-cli` and `crowdstrike-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Match the official Falcon CLIs feature-for-feature on alerts, devices, incidents, Spotlight vulnerabilities, prevention policies, and MSSP Flight Control  -  then go beyond them. fleet sync pulls every child tenant into one local SQLite store keyed by CID, so fleet scorecard, fleet vulns, fleet stale, and fleet policy-drift answer cross-tenant questions instantly and offline, with agent-native JSON on every command.

## When to Use This CLI

Reach for this CLI when an agent or operator needs to inspect or act on a CrowdStrike Falcon estate  -  especially an MSP/MSSP estate spanning many child CIDs. It is the right tool for cross-tenant posture questions (vulnerabilities, stale sensors, policy drift, alert triage) that would otherwise require dozens of live, paginated API calls, and for any scripted alert/device/incident/policy action with agent-native JSON output.

## Anti-triggers

Do not use this CLI for:
- Real-Time Response (RTR) shell sessions, file get/put, or script execution - not implemented; use Falcon-Toolkit or the Falcon console
- Sensor install/uninstall or update rollouts - use your RMM or the Falcon console
- Streaming detections into a SIEM - use the CrowdStrike SIEM connector / Falcon Data Replicator
- Legacy /detects/* Detects API workflows - decommissioned upstream; the Alerts API commands replace it
- Cross-tenant fleet rollups without a parent-CID Flight Control API client - they degrade to single-CID coverage

## Unique Capabilities

### Cross-tenant fleet intelligence
- **`fleet sync`**  -  Pull hosts, alerts, vulnerabilities, policies, and the Flight Control fabric from every child tenant into one local store keyed by CID. Requires a parent-CID API client with Flight Control (MSSP) scope; without it, sync degrades to the single authenticated CID.

  _Reach for this first: it builds the offline fleet store every other fleet command reads from._

  ```bash
  crowdstrike-cli fleet sync --all-cids --json
  ```
- **`fleet scorecard`**  -  One posture board across all tenants: host count, sensor coverage, open critical alerts, critical vulns, and policy posture per CID.

  _Use when an agent needs a whole-book-of-business health summary without dozens of live calls._

  ```bash
  crowdstrike-cli fleet scorecard --json
  ```
- **`fleet vulns`**  -  Rank and filter Spotlight vulnerabilities across every tenant at once, by severity or CVE.

  _Use to answer 'every critical exposure across all customers' in one shot._

  ```bash
  crowdstrike-cli fleet vulns --severity critical --json
  ```
- **`fleet stale`**  -  Find hosts across all tenants whose sensor has not checked in within N days.

  _Use to catch coverage gaps (offline sensors) before they become blind spots._

  ```bash
  crowdstrike-cli fleet stale --days 14 --json
  ```
- **`fleet policy-drift`**  -  Diff each tenant's prevention policy settings against a baseline CID to surface under-protected tenants.

  _Use to enforce a security baseline across a multi-tenant book._

  ```bash
  crowdstrike-cli fleet policy-drift --json
  ```
- **`fleet alerts`**  -  One severity-sorted new-alert queue spanning all tenants, from the local store.

  _The MSP analyst's morning view: what needs attention across every customer, ranked._

  ```bash
  crowdstrike-cli fleet alerts --status new --json
  ```
- **`fleet tenants`**  -  See the whole Flight Control fabric - every child CID, its CID groups, user groups, and role grants - as one offline roster.

  _Reach for this when you need the multi-tenant RBAC and tenant roster without walking five live MSSP endpoints._

  ```bash
  crowdstrike-cli fleet tenants --agent
  ```
- **`fleet remediate`**  -  Group fleet-wide exposure by remediation action - which single fix clears the most hosts and tenants.

  _Use this to turn a vulnerability list into a patch worklist ranked by cross-tenant blast radius._

  ```bash
  crowdstrike-cli fleet remediate --severity critical --agent
  ```
- **`fleet trend`**  -  See which tenants got worse since the last sync - week-over-week deltas in open critical alerts and critical vulnerabilities.

  _Use this for degradation detection across the book of business - the live API only ever shows current state._

  ```bash
  crowdstrike-cli fleet trend --agent
  ```

### Offline intelligence
- **`fleet search`**  -  Full-text search across synced hosts, alerts, vulnerabilities, and policies across all tenants, instantly and offline.

  _Use for instant lookups without burning live API quota or waiting on pagination._

  ```bash
  crowdstrike-cli fleet search ransomware --json --select cid,kind,name
  ```

## Command Reference

**alerts**  -  Detections/alerts (modern Alerts API, replaces the decommissioned Detects API)

- `crowdstrike-cli alerts get-queries-v2`  -  Retrieves all Alerts ids that match a given query.
- `crowdstrike-cli alerts patch-entities-v3`  -  Perform actions on Alerts identified by composite ID(s) in request.
- `crowdstrike-cli alerts post-aggregates-v2`  -  Retrieves aggregate values for Alerts across all CIDs.
- `crowdstrike-cli alerts post-combined-v1`  -  Retrieves all Alerts that match a particular FQL filter.
- `crowdstrike-cli alerts post-entities-v2`  -  Retrieves all Alerts given their composite ids.

**devices**  -  Manage devices

- `crowdstrike-cli devices combined-by-filter`  -  Search for hosts in your environment by platform, hostname, IP, and other criteria. Returns full device records.
- `crowdstrike-cli devices combined-hidden-by-filter`  -  Search for hidden hosts in your environment by platform, hostname, IP, and other criteria. Returns full device records.
- `crowdstrike-cli devices create-host-groups`  -  Create Host Groups by specifying details about the group to create
- `crowdstrike-cli devices delete-host-groups`  -  Delete a set of Host Groups by specifying their IDs
- `crowdstrike-cli devices entities-perform-action`  -  Performs the specified action on the provided group IDs.
- `crowdstrike-cli devices get-details-v2`  -  Get details on one or more hosts by providing host IDs as a query parameter. Supports up to a maximum 100 IDs.
- `crowdstrike-cli devices get-host-groups`  -  Retrieve a set of Host Groups by specifying their IDs
- `crowdstrike-cli devices get-online-state-v1`  -  Get the online status for one or more hosts by specifying each host’s unique ID.
- `crowdstrike-cli devices perform-action-v2`  -  Take various actions on the hosts in your environment. Contain or lift containment on a host. Delete or restore a host.
- `crowdstrike-cli devices perform-group-action`  -  Perform the specified action on the Host Groups specified in the request
- `crowdstrike-cli devices post-details-v2`  -  Get details on one or more hosts by providing host IDs in a POST body. Supports up to a maximum 5000 IDs.
- `crowdstrike-cli devices query-by-filter`  -  Search for hosts in your environment by platform, hostname, IP, and other criteria.
- `crowdstrike-cli devices query-by-filter-scroll`  -  Search for hosts in your environment by platform, hostname, IP
- `crowdstrike-cli devices query-combined-group-members`  -  Search for members of a Host Group in your environment by providing an FQL filter and paging details.
- `crowdstrike-cli devices query-combined-host-groups`  -  Search for Host Groups in your environment by providing an FQL filter and paging details.
- `crowdstrike-cli devices query-get-network-address-history-v1`  -  Retrieve history of IP and MAC addresses of devices.
- `crowdstrike-cli devices query-group-members`  -  Search for members of a Host Group in your environment by providing an FQL filter and paging details.
- `crowdstrike-cli devices query-hidden`  -  Retrieve hidden hosts that match the provided filter criteria.
- `crowdstrike-cli devices query-host-groups`  -  Search for Host Groups in your environment by providing an FQL filter and paging details.
- `crowdstrike-cli devices query-login-history-v2`  -  Retrieve details about recent interactive login sessions for a set of devices powered by the Host Timeline.
- `crowdstrike-cli devices update-host-groups`  -  Update Host Groups by specifying the ID of the group and details to update
- `crowdstrike-cli devices update-tags`  -  Append or remove one or more Falcon Grouping Tags on one or more hosts. Tags must be of the form FalconGroupingTags/

**incidents**  -  Incidents and behaviors: cross-detection correlation and triage

- `crowdstrike-cli incidents crowd-score`  -  DEPRECATED: the incidentapi will be removed in March 2026. Query environment wide CrowdScore and return the entity data
- `crowdstrike-cli incidents get`  -  DEPRECATED: the incidentapi will be removed in March 2026. Get details on incidents by providing incident IDs
- `crowdstrike-cli incidents get-behaviors`  -  DEPRECATED: the incidentapi will be removed in March 2026. Get details on behaviors by providing behavior IDs
- `crowdstrike-cli incidents perform-action`  -  DEPRECATED: the incidentapi will be removed in March 2026.
- `crowdstrike-cli incidents query`  -  DEPRECATED: the incidentapi will be removed in March 2026.
- `crowdstrike-cli incidents query-behaviors`  -  DEPRECATED: the incidentapi will be removed in March 2026.

**mssp**  -  Manage mssp

- `crowdstrike-cli mssp add-cidgroup-members`  -  Add new CID group member.
- `crowdstrike-cli mssp add-role`  -  Create a link between user group and CID group, with zero or more additional roles.
- `crowdstrike-cli mssp add-user-group-members`  -  Add new user group member. Maximum 500 members allowed per user group.
- `crowdstrike-cli mssp create-cidgroups`  -  Create new CID groups. Name is a required field but description is an optional field. Maximum 500 CID groups allowed.
- `crowdstrike-cli mssp create-user-groups`  -  Create new user groups. Name is a required field but description is an optional field.
- `crowdstrike-cli mssp delete-cidgroup-members-v2`  -  Delete CID group members. Prevents removal of a cid group a cid group if it is only part of one cid group.
- `crowdstrike-cli mssp delete-cidgroups`  -  Delete CID groups by ID.
- `crowdstrike-cli mssp delete-user-group-members`  -  Delete user group members entry.
- `crowdstrike-cli mssp delete-user-groups`  -  Delete user groups by ID.
- `crowdstrike-cli mssp deleted-roles`  -  Delete links or additional roles between user groups and CID groups.
- `crowdstrike-cli mssp get-children`  -  Get link to child customer by child CID(s)
- `crowdstrike-cli mssp get-children-v2`  -  Get link to child customer by child CID(s)
- `crowdstrike-cli mssp get-cidgroup-by-id-v2`  -  Get CID Groups by ID.
- `crowdstrike-cli mssp get-cidgroup-members-by-v2`  -  Get CID group members by CID Group ID.
- `crowdstrike-cli mssp get-roles-by-id`  -  Get link between user group and CID group by ID.
- `crowdstrike-cli mssp get-user-group-members-by-idv2`  -  Get user group members by user group ID.
- `crowdstrike-cli mssp get-user-groups-by-idv2`  -  Get user groups by ID.
- `crowdstrike-cli mssp query-children`  -  Query for customers linked as children
- `crowdstrike-cli mssp query-cidgroup-members`  -  Query a CID groups members by associated CID.
- `crowdstrike-cli mssp query-cidgroups`  -  Query CID groups.
- `crowdstrike-cli mssp query-roles`  -  Query links between user groups and CID groups. At least one of CID group ID or user group ID should also be provided.
- `crowdstrike-cli mssp query-user-group-members`  -  Query user group member by user UUID.
- `crowdstrike-cli mssp query-user-groups`  -  Query user groups.
- `crowdstrike-cli mssp update-cidgroups`  -  Update existing CID groups. CID group ID is expected for each CID group definition provided in request body.
- `crowdstrike-cli mssp update-user-groups`  -  Update existing user group(s). User group ID is expected for each user group definition provided in request body.

**policy**  -  Manage policy

- `crowdstrike-cli policy create-prevention-policies`  -  Create Prevention Policies by specifying details about the policy to create
- `crowdstrike-cli policy delete-prevention-policies`  -  Delete a set of Prevention Policies by specifying their IDs
- `crowdstrike-cli policy get-prevention-policies`  -  Retrieve a set of Prevention Policies by specifying their IDs
- `crowdstrike-cli policy perform-prevention-policies-action`  -  Perform the specified action on the Prevention Policies specified in the request
- `crowdstrike-cli policy query-combined-prevention-members`  -  Search for members of a Prevention Policy in your environment by providing an FQL filter and paging details.
- `crowdstrike-cli policy query-combined-prevention-policies`  -  Search for Prevention Policies in your environment by providing an FQL filter and paging details.
- `crowdstrike-cli policy query-prevention-members`  -  Search for members of a Prevention Policy in your environment by providing an FQL filter and paging details.
- `crowdstrike-cli policy query-prevention-policies`  -  Search for Prevention Policies in your environment by providing an FQL filter and paging details.
- `crowdstrike-cli policy set-prevention-policies-precedence`  -  Sets the precedence of Prevention Policies based on the order of IDs specified in the request.
- `crowdstrike-cli policy update-prevention-policies`  -  Update Prevention Policies by specifying the ID of the policy and details to update

**spotlight**  -  Manage spotlight

- `crowdstrike-cli spotlight combined-query-installed-patches`  -  Gets installed patches information for hosts.
- `crowdstrike-cli spotlight combined-query-vulnerabilities`  -  Search for Vulnerabilities in your environment by providing an FQL filter and paging details.
- `crowdstrike-cli spotlight get-remediations`  -  Get details on remediations by providing one or more IDs
- `crowdstrike-cli spotlight get-vulnerabilities`  -  Get details on vulnerabilities by providing one or more IDs
- `crowdstrike-cli spotlight query-vulnerabilities`  -  Search for Vulnerabilities in your environment by providing an FQL filter and paging details.


**fleet**


- `crowdstrike-cli fleet sync`  -  Pull hosts, alerts, vulnerabilities, policies, and the Flight Control fabric from every child tenant into one local store keyed by CID. Requires a parent-CID API client with Flight Control (MSSP) scope; without it, sync degrades to the single authenticated CID.

- `crowdstrike-cli fleet scorecard`  -  One posture board across all tenants: host count, sensor coverage, open critical alerts, critical vulns, and policy posture per CID.

- `crowdstrike-cli fleet vulns`  -  Rank and filter Spotlight vulnerabilities across every tenant at once, by severity or CVE.

- `crowdstrike-cli fleet stale`  -  Find hosts across all tenants whose sensor has not checked in within N days.

- `crowdstrike-cli fleet policy-drift`  -  Diff each tenant's prevention policy settings against a baseline CID to surface under-protected tenants.

- `crowdstrike-cli fleet alerts`  -  One severity-sorted new-alert queue spanning all tenants, from the local store.

- `crowdstrike-cli fleet search`  -  Full-text search across synced hosts, alerts, vulnerabilities, and policies across all tenants, instantly and offline.

- `crowdstrike-cli fleet tenants`  -  See the whole Flight Control fabric - every child CID, its CID groups, user groups, and role grants - as one offline roster.

- `crowdstrike-cli fleet remediate`  -  Group fleet-wide exposure by remediation action - which single fix clears the most hosts and tenants.

- `crowdstrike-cli fleet trend`  -  See which tenants got worse since the last sync - week-over-week deltas in open critical alerts and critical vulnerabilities.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
crowdstrike-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Whole-fleet critical exposure

```bash
crowdstrike-cli fleet vulns --severity critical --agent --select cid,cve,hostname,severity
```

Pulls every critical Spotlight vuln across all tenants from the store and narrows to the fields an agent needs.

### Morning triage queue

```bash
crowdstrike-cli fleet alerts --status new --agent --select cid,severity,name,device.hostname
```

One severity-sorted new-alert queue across every tenant, field-narrowed for agent context.

### Coverage audit

```bash
crowdstrike-cli fleet stale --days 30 --json
```

Lists hosts whose sensor has gone dark for 30+ days across the whole fleet.

### Baseline enforcement

```bash
crowdstrike-cli fleet policy-drift --json
```

Surfaces tenants whose prevention policy is weaker than the baseline CID.

### Tenant coverage at a glance

```bash
crowdstrike-cli fleet scorecard --agent --select cid,coverage_pct,critical_vulns,open_critical_alerts
```

Narrows the per-tenant posture board to the fields an agent needs to triage the fleet.

## Auth Setup

Auth is OAuth2 client_credentials. Create an API client in the Falcon console (Support > API Clients & Keys) with scopes for the resources you use, then set FALCON_CLIENT_ID and FALCON_CLIENT_SECRET and run 'crowdstrike-cli auth login' to mint and cache a bearer token (auto-refreshed before expiry). For MSSP/Flight Control, a single parent-CID client operates on child tenants by minting a member_cid-scoped token; the fleet commands handle this for you, minting a per-tenant token for each child CID (or for a single tenant you name). Pick your cloud by overriding the base/token URL (US-1 default, US-2, EU-1, or GovCloud).

Run `crowdstrike-cli doctor` to verify setup.

## Credential safety

- Every API request and the bearer token go only to the scheme, host and port of `CROWDSTRIKE_BASE_URL` (default `https://api.crowdstrike.com`, US-1; set it to `https://api.us-2.crowdstrike.com`, `https://api.eu-1.crowdstrike.com` or `https://api.laggar.gcw.crowdstrike.com` for another cloud). The client ID and secret go only to the token URL: `CROWDSTRIKE_TOKEN_URL`, else `/oauth2/token` on that same cloud host, so an EU-1 or GovCloud client never posts its secret to US-1. A redirect to any other host is refused, so a credential never reaches it.
- Any output, file, request, SQL statement, child process or MCP message that would repeat `FALCON_CLIENT_ID`, `FALCON_CLIENT_SECRET`, a minted token (Flight Control member_cid tokens included), `CROWDSTRIKE_MCP_HTTP_TOKEN` or a credential-named value from a response is withheld whole and replaced by a notice such as "[credkit: withheld from stdout: it carried a credential this program holds]". The same goes for 8 or more characters in a row of any of them, in the encodings the check knows, and for the password or query values of a URL setting (`CROWDSTRIKE_BASE_URL`, `CROWDSTRIKE_TOKEN_URL`, `CROWDSTRIKE_FEEDBACK_ENDPOINT`, `base_url` and `token_url` in the config file). Input read with `--stdin` is held as a whole, so a `--dry-run` preview of it prints the notice instead of the body. Responses are never cached on disk, and credential-named fields are dropped before anything reaches the local store.
- Only `credentials.toml` (mode 0600) holds a credential. A client ID and secret supplied by the environment are never written to disk, and nor is the token minted for them: the process keeps it in memory (the MCP server reuses it until it expires). A token is cached in `credentials.toml` only beside the client pair stored there (not at all under `CROWDSTRIKE_NO_CONFIG_WRITE=1`), and a token is only ever sent for the client and cloud it was minted for. `fleet sync` keeps each child CID's member_cid token in memory and sends it only for that CID.
- `--dry-run` sends nothing and never mints a token. Over MCP, a call that changes CrowdStrike (containing a host, deleting a policy or host group) returns a preview and sends only with `confirm: true`, and a tool argument can never name a file to read or write.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, sync, and `--deliver` paths:

- `--json`  -  one JSON document on stdout (sync progress events go to stderr)
- `--compact`  -  keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain`  -  tabular rows (collection envelopes unwrap to the row array). `--csv` prefixes `'` on a cell or header key that starts with `=`, `+`, `-`, `@`, tab, CR, or LF (fullwidth `＝＋－＠` too) so a spreadsheet does not run it as a formula; a plain signed number such as `-5` or `+2.5e3` is unchanged.
- `--quiet`  -  one identity value per row, no envelope

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  crowdstrike-cli incidents get --agent
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit confirmation**  -  `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
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

- Use `--home <dir>` for one invocation, or set `CROWDSTRIKE_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `CROWDSTRIKE_CONFIG_DIR`, `CROWDSTRIKE_DATA_DIR`, `CROWDSTRIKE_STATE_DIR`, `CROWDSTRIKE_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `CROWDSTRIKE_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `crowdstrike-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "crowdstrike": {
        "command": "crowdstrike-mcp",
        "env": {
          "CROWDSTRIKE_HOME": "/srv/crowdstrike"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `CROWDSTRIKE_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `CROWDSTRIKE_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are never cached on disk. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `CROWDSTRIKE_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `CROWDSTRIKE_DATA_SOURCE` sets the `--data-source` default.

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
crowdstrike-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "crowdstrike-cli learnings confirm 12"] }
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
       materially more, record the divergence via `crowdstrike-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `crowdstrike-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `crowdstrike-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
crowdstrike-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
crowdstrike-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
crowdstrike-cli teach-playbook \
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
crowdstrike-cli playbook amend \
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

`crowdstrike-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `CROWDSTRIKE_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
crowdstrike-cli feedback "the --since flag is inclusive but docs say exclusive"
crowdstrike-cli feedback --stdin < notes.txt
crowdstrike-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `CROWDSTRIKE_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `CROWDSTRIKE_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` as a private 0600 file (exclusive randomly named temp in the destination directory, then rename; a planted symlink is not followed). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`). Redirects are not followed; a 3xx response fails the delivery and the body is not sent to the `Location`. |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures, including redirects, return non-zero and log the URL and HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
crowdstrike-cli profile save briefing --json
crowdstrike-cli --profile briefing incidents get
crowdstrike-cli profile list --json
crowdstrike-cli profile show briefing
crowdstrike-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `crowdstrike-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add crowdstrike-mcp -- crowdstrike-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `crowdstrike-mcp --transport http` requires `CROWDSTRIKE_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `CROWDSTRIKE_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `CROWDSTRIKE_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which crowdstrike-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   crowdstrike-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `crowdstrike-cli <command> --help`.
