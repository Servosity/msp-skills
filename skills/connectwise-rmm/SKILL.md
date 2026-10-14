---
name: connectwise-rmm
description: "Use when the user asks to find offline or silent ConnectWise RMM (Asio) agents across clients, rank clients by patch compliance, build the patch-week reboot queue, find which machine a user is logged on to, prepare one client's health card, run a script on endpoints with a preview first, or see why a script run failed. Syncs companies, sites, endpoints and tickets into a local SQLite mirror so cross-client questions answer in one command. Trigger phrases: `which endpoints are offline in ConnectWise RMM`, `patch compliance for a client in Asio`, `run a script on an endpoint with ConnectWise RMM`, `which machine is a user logged on to in ConnectWise RMM`, `list ConnectWise RMM companies`, `use connectwise-rmm`, `run connectwise-rmm`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "ConnectWise"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - connectwise-rmm-cli
    install:
      - kind: script
        bins: [connectwise-rmm-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-rmm/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-rmm/install.ps1
---

# ConnectWise RMM Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `connectwise-rmm-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-rmm/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-rmm/install.ps1 | iex
   ```
3. Verify: `connectwise-rmm-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `connectwise-rmm-cli` and `connectwise-rmm-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Lists companies, sites and endpoints across every client, reads patch state, logged-on users, services and resource usage, and drives the automation engine: run previews a script on named endpoints and, with --confirm, schedules it and collects per-endpoint results. Sync the fleet into SQLite once and the fleet commands roll it up across clients; scopes tells you exactly which OAuth scopes your API key was granted.

## When to Use This CLI

Use this CLI when an agent or technician needs ConnectWise RMM (Asio) fleet data across many client companies: which endpoints exist, which are offline, what is missing patches, who is logged on, what software is installed, or to run a script on endpoints and collect the results. Run sync --resources companies,sites,endpoints,tickets first: the fleet commands read that local copy and check availability, patches and sessions live. Pass endpoint IDs; machine names resolve after fleet whois has cached them. run previews by default and only schedules with --confirm. An empty fleet result with scanned_endpoints: 0 means nothing is synced yet, not that the fleet is healthy. fleet patch-posture, reboot-due, scorecard and whois read up to 500 endpoints by default; --max-endpoints 0 lifts the cap and scan_cap_hit flags a partial result.

## Anti-triggers

Do not use this CLI for:
- ConnectWise Manage (PSA) tickets, agreements, time entries or invoices; this CLI's tickets commands manage ConnectWise RMM (Asio) native service tickets only
- ConnectWise Automate (LabTech) on-prem servers; that is a different product and API
- ConnectWise Control / ScreenConnect remote sessions
- Interactive remote control of a machine; use run to schedule a script instead
- Agent health from the generic top-level stale, load or orphans helpers; use fleet stale for offline and silent agents

## Unique Capabilities

### Fleet health
- **`fleet stale`**  -  Every endpoint across all companies that is offline, missing from the heartbeat, or silent longer than a threshold, sorted by company and silence.

  _Reach for this for 'which agents are offline' across clients instead of looping heartbeat calls yourself._

  ```bash
  connectwise-rmm-cli fleet stale --older-than 24h --agent
  ```
- **`fleet whois`**  -  Which endpoints a person is logged on to now or was last logged on to, with company and site names.

  _Use this when a ticket names a person, not a machine._

  ```bash
  connectwise-rmm-cli fleet whois jsmith --company "Acme Corp" --agent
  ```
- **`fleet brief`**  -  One card for one endpoint: online or last seen, logged-on users, patch counts, and tickets linked to it or its site.

  _Use this first when a ticket mentions one machine._

  ```bash
  connectwise-rmm-cli fleet brief 4a9caa07-d45a-4c14-aa36-8c1c26e7a029 --agent
  ```

### Patch compliance
- **`fleet patch-posture`**  -  Companies ranked by patch risk: endpoints below a compliance bar, missing patches, pending reboots, out of support, stale assessments.

  _Use this for per-company patch questions; the API returns per-endpoint rows with no per-company roll-up._

  ```bash
  connectwise-rmm-cli fleet patch-posture --min-compliance 90 --agent
  ```
- **`fleet scorecard`**  -  Per-site health card for one company: endpoint mix, offline count, patch risk and tickets by status.

  _Use this to prepare a client review._

  ```bash
  connectwise-rmm-cli fleet scorecard "Acme Corp" --agent
  ```
- **`fleet reboot-due`**  -  Endpoints with pending reboots split into reboot now, coordinate (user logged on or sessions unknown), offline, and availability unknown.

  _Use this as the patch-week reboot work queue._

  ```bash
  connectwise-rmm-cli fleet reboot-due --agent
  ```

### Automation
- **`fleet task-failures`**  -  Failed endpoints from a script run grouped by identical error text, joined to endpoint names, with IDs ready to re-run.

  _Use this after a fleet script push to see what failed and why._

  ```bash
  connectwise-rmm-cli fleet task-failures 7c9e6679-7425-40de-944b-e07fc1f90ae7 --agent
  ```

## Command Reference

**companies**  -  Manage companies

- `connectwise-rmm-cli companies get-company`  -  Get a company by ID. Scope: platform.companies.read.
- `connectwise-rmm-cli companies list`  -  List all companies (clients). Scope: platform.companies.read.

**custom_fields**  -  Manage custom fields

- `connectwise-rmm-cli custom-fields get-company`  -  Get custom field values of a company. Scope: platform.custom_fields_values.read.
- `connectwise-rmm-cli custom-fields get-definition`  -  Get one custom field definition. Scope: platform.custom_fields_definitions.read.
- `connectwise-rmm-cli custom-fields get-endpoint`  -  Get custom field values of an endpoint. Scope: platform.custom_fields_values.read.
- `connectwise-rmm-cli custom-fields get-site`  -  Get custom field values of a site. Scope: platform.custom_fields_values.read.
- `connectwise-rmm-cli custom-fields list-definitions`  -  List custom field definitions. Scope: platform.custom_fields_definitions.read.
- `connectwise-rmm-cli custom-fields set-company`  -  Set custom field values on a company. Scope: platform.custom_fields_values.write.
- `connectwise-rmm-cli custom-fields set-endpoint`  -  Set custom field values on an endpoint. Scope: platform.custom_fields_values.write.
- `connectwise-rmm-cli custom-fields set-site`  -  Set custom field values on a site. Scope: platform.custom_fields_values.write.

**device_groups**  -  Manage device groups

- `connectwise-rmm-cli device-groups`  -  List all device groups (static, dynamic, predefined). Scope: platform.deviceGroups.read.

**endpoints**  -  Manage endpoints

- `connectwise-rmm-cli endpoints create-agent-install-token`  -  Get the agent install token for a company and site. Exposed API for Generate Token.
- `connectwise-rmm-cli endpoints get`  -  Get full details (hardware, OS, agent) of one endpoint. Scope: platform.asset.read.
- `connectwise-rmm-cli endpoints get-cpu-usage`  -  Get the latest CPU usage of an endpoint. Latest CPU usage. Required scope is not stated in the public sources.
- `connectwise-rmm-cli endpoints get-disk-usage`  -  Get the latest disk usage of an endpoint. Latest disk usage. Required scope is not stated in the public sources.
- `connectwise-rmm-cli endpoints get-heartbeat`  -  Check agent availability (online/offline, last seen) for up to 400 endpoints. Agent Last Seen API.
- `connectwise-rmm-cli endpoints get-memory-usage`  -  Get the latest memory usage of an endpoint. Latest memory usage; minute sets how many minutes of history to return.
- `connectwise-rmm-cli endpoints get-services`  -  List Windows services or daemons on one endpoint. Service details for an endpoint; filter by serviceName.
- `connectwise-rmm-cli endpoints get-summary`  -  Fetch projected endpoint inventory for companies, sites, or endpoints (Device Summary API). Device Summary API.
- `connectwise-rmm-cli endpoints get-system-state`  -  Get last boot time and logged-on users of an endpoint. Scope: platform.devices.read.
- `connectwise-rmm-cli endpoints list`  -  List every managed endpoint for the partner. Scope: platform.asset.read.
- `connectwise-rmm-cli endpoints list-applications`  -  List installed applications on a set of endpoints. Software inventory.
- `connectwise-rmm-cli endpoints list-company`  -  List the endpoints of one company. Scope: platform.asset.read.
- `connectwise-rmm-cli endpoints list-services`  -  List endpoint services for the partner. Scope: platform.asset.read.
- `connectwise-rmm-cli endpoints list-site`  -  List the endpoints of one site. Scope: platform.asset.read.
- `connectwise-rmm-cli endpoints uninstall`  -  Uninstall (offboard) the agent from one or more endpoints. Scope: platform.agent.delete.

**hypervisors**  -  Manage hypervisors

- `connectwise-rmm-cli hypervisors list-guests`  -  List guest VMs of a hypervisor host. Scope: platform.devices.read.
- `connectwise-rmm-cli hypervisors list-hosts`  -  List monitored hypervisor hosts. Fetch hypervisors Hosts. Scope: platform.devices.read.

**patches**  -  Manage patches

- `connectwise-rmm-cli patches get-endpoint`  -  Get patch compliance summary and patch list for an endpoint. Scope: platform.patching.read.
- `connectwise-rmm-cli patches get-os-patch-compliance`  -  Get OS patch compliance summary for a set of endpoints. Bulk OS patch compliance.
- `connectwise-rmm-cli patches get-os-patch-details`  -  Get OS patch details for a set of endpoints. Bulk OS patch details. Body {resourceType: endpoints, resources: [ids]}.
- `connectwise-rmm-cli patches get-os-patch-inventory`  -  Get the partner-wide OS patch inventory. Every OS patch known across the partner.
- `connectwise-rmm-cli patches get-third-party-patch-compliance`  -  Get third-party (non-OS) patch compliance for a set of endpoints. Bulk third-party application patch compliance.
- `connectwise-rmm-cli patches get-third-party-patch-details`  -  Get third-party (non-OS) patch details for a set of endpoints. Bulk third-party application patch details.

**policies**  -  Manage policies

- `connectwise-rmm-cli policies get-effective-policy`  -  Get the effective policy applied to an endpoint. Scope: platform.policies.read.
- `connectwise-rmm-cli policies get-effective-policy-override`  -  Get policy overrides for an endpoint. Scope: platform.policies.read.
- `connectwise-rmm-cli policies list-policy-categories`  -  List policy categories and subcategories. Scope: platform.policies.read.

**rate_limits**  -  Manage rate limits

- `connectwise-rmm-cli rate-limits`  -  Get current rate limit.

**scripts**  -  Manage scripts

- `connectwise-rmm-cli scripts list`  -  List scripts created in the script editor (templateType fusionscript). Scope: platform.automation.read.
- `connectwise-rmm-cli scripts list-custom-templates`  -  List custom task templates (templateType custom). Scope: platform.automation.read.
- `connectwise-rmm-cli scripts list-shell`  -  List out-of-the-box global script templates (templateType script). Scope: platform.automation.read.

**sites**  -  Manage sites

- `connectwise-rmm-cli sites create`  -  Create a site under a company. Scope: platform.sites.write.
- `connectwise-rmm-cli sites get`  -  Get a site by ID. Scope: platform.sites.read.
- `connectwise-rmm-cli sites list`  -  List all sites across every company. Scope: platform.sites.read.
- `connectwise-rmm-cli sites list-company`  -  List the sites of one company. Scope: platform.sites.read.
- `connectwise-rmm-cli sites update`  -  Partially update a site. Scope: platform.sites.write.

**tasks**  -  Manage tasks

- `connectwise-rmm-cli tasks get-instance-results`  -  Get per-endpoint results of one task run. Scope: platform.automation.read.
- `connectwise-rmm-cli tasks get-instance-summary`  -  Get the run history (instances) of a task. Scope: platform.automation.read.
- `connectwise-rmm-cli tasks list-endpoint-summaries`  -  List tasks that ran on one endpoint. Scope: platform.automation.read.
- `connectwise-rmm-cli tasks list-summaries`  -  List all scheduled tasks (the Scheduled Tasks page). Scope: platform.automation.read.
- `connectwise-rmm-cli tasks schedule`  -  Schedule a script or template to run on target endpoints. Scope: platform.automation.create.

**ticket_notes**  -  Manage ticket notes

- `connectwise-rmm-cli ticket-notes create`  -  Add a note to a ticket. This API creates a new note for the specified ticket in the system.
- `connectwise-rmm-cli ticket-notes list`  -  List notes on a ticket. This API fetches the selected notes for a specific ticket from system and returns it.

**ticket_settings**  -  Manage ticket settings

- `connectwise-rmm-cli ticket-settings list-service-board-teams`  -  List teams of a service board. This API fetches all the team associations for a specific service-board.
- `connectwise-rmm-cli ticket-settings list-service-boards`  -  List service boards.
- `connectwise-rmm-cli ticket-settings list-ticket-categories`  -  List ticket categories. This API returns all the ticket categories for a partner.
- `connectwise-rmm-cli ticket-settings list-ticket-priorities`  -  List ticket priorities. This API returns all the available priorities in the system.
- `connectwise-rmm-cli ticket-settings list-ticket-sources`  -  List ticket sources. This API returns all the ticket sources for a partner.
- `connectwise-rmm-cli ticket-settings list-ticket-statuses`  -  List ticket statuses. This API returns all the available statuses in the system.
- `connectwise-rmm-cli ticket-settings list-ticket-tags`  -  List ticket tags. This API returns all the available tags in the system, for partner with provided partnerId.
- `connectwise-rmm-cli ticket-settings list-ticket-types`  -  List ticket types. This API returns all the available types in the system.

**tickets**  -  Manage tickets

- `connectwise-rmm-cli tickets create`  -  Create a service ticket. This API creates a new ticket in the system.
- `connectwise-rmm-cli tickets get`  -  Get a service ticket. Fetches the ticket stored in the system against the provided id.
- `connectwise-rmm-cli tickets get-counts`  -  Get ticket counts. This API returns ticket counts
- `connectwise-rmm-cli tickets list`  -  List service tickets (filterable; page by passing --page-size and --page-num together).
- `connectwise-rmm-cli tickets update`  -  Update a service ticket with JSON Patch operations.


**fleet**


- `connectwise-rmm-cli fleet stale`  -  Every endpoint across all companies that is offline, missing from the heartbeat, or silent longer than a threshold, sorted by company and silence.

- `connectwise-rmm-cli fleet patch-posture`  -  Companies ranked by patch risk: endpoints below a compliance bar, missing patches, pending reboots, out of support, stale assessments.

- `connectwise-rmm-cli fleet whois`  -  Which endpoints a person is logged on to now or was last logged on to, with company and site names.

- `connectwise-rmm-cli fleet brief`  -  One card for one endpoint: online or last seen, logged-on users, patch counts, and tickets linked to it or its site.

- `connectwise-rmm-cli fleet scorecard`  -  Per-site health card for one company: endpoint mix, offline count, patch risk and tickets by status.

- `connectwise-rmm-cli fleet task-failures`  -  Failed endpoints from a script run grouped by identical error text, joined to endpoint names, with IDs ready to re-run.

- `connectwise-rmm-cli fleet reboot-due`  -  Endpoints with pending reboots split into reboot now, coordinate (user logged on or sessions unknown), offline, and availability unknown.




**run and scopes**

- `connectwise-rmm-cli run`  -  Resolve a script and endpoints by name or ID and print the exact schedule request; `--confirm` schedules it, `--wait` polls and returns per-endpoint results.
- `connectwise-rmm-cli scopes`  -  Mint a token per candidate scope, report granted and denied scopes, and print the CONNECTWISE_RMM_OAUTH_SCOPE line to export.

### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
connectwise-rmm-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Monday offline sweep

```bash
connectwise-rmm-cli fleet stale --older-than 48h --type Server --agent
```

Offline servers plus servers silent for over 48 hours or missing from the heartbeat, sorted by company and silence.

### Patch numbers for one endpoint

```bash
connectwise-rmm-cli patches get-endpoint 4a9caa07-d45a-4c14-aa36-8c1c26e7a029 --agent --select summary.compliance,summary.missingCount,summary.pendingRebootCount,summary.policyName
```

Narrow the patch payload to the four numbers a client review needs.

### Which machines is this user on

```bash
connectwise-rmm-cli fleet whois jsmith --company "Acme Corp" --agent
```

Every endpoint where a user is logged on now or was the last user, with company and site names.

### Reboot queue

```bash
connectwise-rmm-cli fleet reboot-due --agent
```

Pending-reboot endpoints split into reboot now, coordinate (user logged on or sessions unknown), offline, and availability unknown.

### Run a script safely

```bash
connectwise-rmm-cli run --script "Restart Print Spooler" --endpoint 4a9caa07-d45a-4c14-aa36-8c1c26e7a029 --agent
```

Preview only: resolves the script and targets and prints the exact request. Add --confirm --wait to schedule it and collect per-endpoint results.

### Why did the script fail

```bash
connectwise-rmm-cli fleet task-failures 7c9e6679-7425-40de-944b-e07fc1f90ae7 --agent
```

Failed endpoints grouped by identical error text, with endpoint IDs ready to pass back to run.

## Auth Setup

ConnectWise RMM uses OAuth2 client credentials. Create an API key under Integrations > API Access in the ConnectWise portal, then export `CONNECTWISE_RMM_CLIENT_ID` and `CONNECTWISE_RMM_CLIENT_SECRET`. The CLI mints and caches the bearer token itself by posting a JSON body to `<base_url>/v1/token`. The client ID prefix picks the region (0e30 North America, 0e31 Europe, 0e32 Australia); set `CONNECTWISE_RMM_BASE_URL` to override. Run `connectwise-rmm-cli scopes --agent` and export the `CONNECTWISE_RMM_OAUTH_SCOPE` line it prints, because one ungranted scope fails the whole token request. `connectwise-rmm-cli auth login` mints and caches a token explicitly.

Run `connectwise-rmm-cli doctor` to verify setup.

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
  connectwise-rmm-cli companies list --agent --select account,acquiredDate,activityStatus
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit confirmation**  -  `--agent` does not imply confirmation. Generated write commands (sites create/update, tickets create/update, ticket-notes create, custom-fields set-*, tasks schedule, endpoints uninstall, endpoints create-agent-install-token, import) run immediately, so preview them with `--dry-run`; `run` only schedules when `--confirm` is passed
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

- Use `--home <dir>` for one invocation, or set `CONNECTWISE_RMM_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `CONNECTWISE_RMM_CONFIG_DIR`, `CONNECTWISE_RMM_DATA_DIR`, `CONNECTWISE_RMM_STATE_DIR`, `CONNECTWISE_RMM_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `CONNECTWISE_RMM_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `connectwise-rmm-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "connectwise-rmm": {
        "command": "connectwise-rmm-mcp",
        "env": {
          "CONNECTWISE_RMM_HOME": "/srv/connectwise-rmm"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `CONNECTWISE_RMM_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `CONNECTWISE_RMM_HOME`, or `doctor` will not find credentials left under the former root.

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
connectwise-rmm-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "connectwise-rmm-cli learnings confirm 12"] }
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
       materially more, record the divergence via `connectwise-rmm-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `connectwise-rmm-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `connectwise-rmm-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
connectwise-rmm-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
connectwise-rmm-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
connectwise-rmm-cli teach-playbook \
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
connectwise-rmm-cli playbook amend \
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

`playbook amend` notes are designed to potentially flow upstream as shared knowledge in future versions of this CLI. Keep them clean of user-identifying content so the upstream-contribution path stays open without retroactive scrubbing:

- **Do NOT embed** paths to user filesystems, personal API keys or tokens, user email addresses, user GitHub handles, or specific query histories tied to a single user.
- **Acceptable**: endpoint shapes, undocumented field names, API gotchas, observed schema drift, workarounds for CLI surfaces, generalizable pagination or retry tactics.

If a correction is only meaningful with user-specific context, it belongs in a personal note, not in the playbook amend.

### Measuring the loop

`connectwise-rmm-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `CONNECTWISE_RMM_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
connectwise-rmm-cli feedback "the --since flag is inclusive but docs say exclusive"
connectwise-rmm-cli feedback --stdin < notes.txt
connectwise-rmm-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `CONNECTWISE_RMM_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `CONNECTWISE_RMM_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
connectwise-rmm-cli profile save briefing --json
connectwise-rmm-cli --profile briefing companies list
connectwise-rmm-cli profile list --json
connectwise-rmm-cli profile show briefing
connectwise-rmm-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `connectwise-rmm-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add connectwise-rmm-mcp --env CONNECTWISE_RMM_CLIENT_ID="$CONNECTWISE_RMM_CLIENT_ID" --env CONNECTWISE_RMM_CLIENT_SECRET="$CONNECTWISE_RMM_CLIENT_SECRET" --env CONNECTWISE_RMM_OAUTH_SCOPE="$CONNECTWISE_RMM_OAUTH_SCOPE" -- connectwise-rmm-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which connectwise-rmm-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   connectwise-rmm-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `connectwise-rmm-cli <command> --help`.
