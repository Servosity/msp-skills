---
name: netsapiens
description: "Every NetSapiens v2 endpoint plus a reseller-wide fleet view: seat limits, offline phones, unrouted numbers and E911 gaps across every client at once. Trigger phrases: `which phones are offline`, `who owns this DID`, `which clients are over their seat limit`, `check E911 across our VoIP clients`, `missed calls report for netsapiens`, `use netsapiens`, `run netsapiens`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "NetSapiens"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - netsapiens-cli
    install:
      - kind: script
        bins: [netsapiens-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/netsapiens/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/netsapiens/install.ps1
---

# NetSapiens - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `netsapiens-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/netsapiens/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/netsapiens/install.ps1 | iex
   ```
3. Verify: `netsapiens-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `netsapiens-cli` and `netsapiens-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Works against any white-label NetSapiens platform (Crexendo, OITVOIP, PacketDial and others) with one API key. Sync your reseller's domains, users, devices and numbers into a local SQLite store, then run fleet audits such as fleet seats, fleet offline, fleet dids and fleet e911, or trace a number to its client and phones in one call.

## When to Use This CLI

Use this CLI when an MSP or telecom reseller needs answers across many NetSapiens client domains at once: seat utilization, offline phones, number routing, E911 gaps, missed calls and queue abandonment. It also covers every v2 endpoint for single-domain reads and moves, adds and changes.

## Anti-triggers

Do not use this CLI for:
- Do not use it as a softphone: it carries no audio. The call-control commands (domains users calls-create, calls-answer, calls-transfer, calls-delete) act on real live calls, so confirm before running them.
- Do not use it for NetSapiens API v1 (OAuth-only legacy) integrations.
- Do not use it to provision phones by pushing configuration files; it reads and writes NetSapiens records only.
- Do not use it for billing invoices; it reports counts, not prices.

## Unique Capabilities

### Fleet audits across every tenant
- **`fleet seats`**  -  From the synced mirror (run sync first): each client domain's users, call queues, auto attendants and conferences against their limits, plus device, number, call and SMS counts, with over-limit and near-limit domains flagged and a flat true-up CSV.

  _Reach for this when asked which clients are over or near their seat limits or for monthly true-up numbers._

  ```bash
  netsapiens-cli fleet seats --agent
  ```
- **`fleet offline`**  -  From the synced mirror (run sync first): phones and softphones that are unregistered, expired, high-latency, never provisioned or not provisioned recently, sorted by client.

  _Use this for 'which phones are down' sweeps across all clients instead of checking one domain at a time._

  ```bash
  netsapiens-cli fleet offline --agent
  ```
- **`fleet dids`**  -  From the synced mirror (run sync first): where every phone number routes, flagging numbers whose target is missing for that routing type, disabled or parked numbers, unverifiable targets, and possible dead ends (a user with no registered phone and voicemail off; answer rules are not inspected).

  _Use this after onboarding or porting numbers to catch numbers that route nowhere._

  ```bash
  netsapiens-cli fleet dids --problems-only --agent
  ```
- **`fleet e911`**  -  From the synced mirror (run sync first): domains, users and devices with no emergency caller ID after inheritance, or with an emergency address ID that does not exist. A data-gap check to feed a Kari's Law / RAY BAUM's Act review, not a full compliance test.

  _Use this to find E911 data gaps across every client before a compliance review._

  ```bash
  netsapiens-cli fleet e911 --agent
  ```
- **`fleet changes`**  -  From the synced mirror: users, devices, numbers and domains added, removed or re-pointed since the baseline. Read-only; record or move the baseline with --rebaseline.

  _Use this for 'what changed across clients since the last check'; run with --rebaseline at each check to move the baseline._

  ```bash
  netsapiens-cli fleet changes --agent
  ```

### Ticket triage
- **`trace`**  -  From the synced mirror (run sync first): resolve one phone number, extension, MAC address or email to its client, routing, user, and the user's phones with their registration state.

  _Use this first on a support ticket that names only a number, extension or MAC._

  ```bash
  netsapiens-cli trace 6025550142 --agent
  ```

### Call-handling reviews
- **`fleet missed`**  -  Live (needs the API key): unanswered inbound calls per user and client over a window, read from CDRs; adds names and registered-phone counts when users and devices are synced.

  _Use this for weekly client reviews of missed direct calls._

  ```bash
  netsapiens-cli fleet missed --window 7d --agent
  ```
- **`fleet queues`**  -  Live (needs the API key): call queues across clients ranked by abandonment rate, with service level, available agents and agents whose phones are not registered. Checks up to --max-domains (25) domains and reports when it was capped.

  _Use this when asked which clients' queues are dropping callers and why._

  ```bash
  netsapiens-cli fleet queues --window 7d --agent
  ```

## Command Reference

**accesslog**  -  Manage accesslog

- `netsapiens-cli accesslog`  -  Read Accesslog

**apikeys**  -  Manage apikeys

- `netsapiens-cli apikeys create`  -  This is a limited action and will require special access to create API keys.
- `netsapiens-cli apikeys delete`  -  This is a limited action and will require special access to revoke API keys.
- `netsapiens-cli apikeys get`  -  This action will show apikeys that have been generated by your current APIkey and that you have access to update or
- `netsapiens-cli apikeys list`  -  This action will show apikeys that have been generated by your current APIkey and that you have access to update or
- `netsapiens-cli apikeys my`  -  This parameter-less action will return the information on the API key being used for the request giving confirmation on
- `netsapiens-cli apikeys update`  -  This is a limited action and will require special access to create API keys.

**auditlog**  -  Manage auditlog

- `netsapiens-cli auditlog`  -  Read Audit Log

**auth-code**  -  Manage auth code

- `netsapiens-cli auth-code`  -  Get Access Token From Auth Code

**backup**  -  Manage backup

- `netsapiens-cli backup`  -  Request a Full System backup

**calls**  -  Manage calls

- `netsapiens-cli calls`  -  Report Active Calls

**cdrs**  -  Manage cdrs

- `netsapiens-cli cdrs count`  -  Count CDRs and SUM minutes
- `netsapiens-cli cdrs list`  -  Read CDRs

**certificates**  -  Manage certificates

- `netsapiens-cli certificates create`  -  Create SSL certificate for CertManager
- `netsapiens-cli certificates delete`  -  Delete SSL certificate for CertManager
- `netsapiens-cli certificates get`  -  Read SSL certificate by Common Name
- `netsapiens-cli certificates list`  -  Read SSL certificates for CertManager
- `netsapiens-cli certificates update`  -  Update SSL certificate for CertManager

**config-definitions**  -  Manage config definitions

- `netsapiens-cli config-definitions create`  -  Create a Configuration Definition
- `netsapiens-cli config-definitions delete`  -  This will delete the configuration definition and all configurations that match the name.
- `netsapiens-cli config-definitions get`  -  Read a Specific Configuration Definition
- `netsapiens-cli config-definitions list`  -  Read all Configuration Definitions
- `netsapiens-cli config-definitions update`  -  Update a Configuration Definition

**configurations**  -  Manage configurations

- `netsapiens-cli configurations count`  -  Count Configurations
- `netsapiens-cli configurations create`  -  Create a Configuration
- `netsapiens-cli configurations delete`  -  Delete a Configuration
- `netsapiens-cli configurations get`  -  Read a Specific Configuration
- `netsapiens-cli configurations list`  -  Read all Configurations
- `netsapiens-cli configurations update`  -  Update a configurution, must already exist or will get a 404.

**connections**  -  Manage connections

- `netsapiens-cli connections count`  -  Count All Conections
- `netsapiens-cli connections create`  -  Create a Connection
- `netsapiens-cli connections list`  -  Get All Connections

**deviceprofiles**  -  Manage deviceprofiles

- `netsapiens-cli deviceprofiles count`  -  Count Device Profiles
- `netsapiens-cli deviceprofiles get-by-model`  -  Get specific Device Profile for Model of Phone
- `netsapiens-cli deviceprofiles list`  -  Read Device Profiles

**dialplans**  -  Manage dialplans

- `netsapiens-cli dialplans create`  -  Create Dialplan Global
- `netsapiens-cli dialplans list`  -  Read Dialplans

**dialpolicy**  -  Manage dialpolicy

- `netsapiens-cli dialpolicy create`  -  Create Dialpolicy Table
- `netsapiens-cli dialpolicy delete`  -  Delete Dialpolicy
- `netsapiens-cli dialpolicy get`  -  Read Specific Policy
- `netsapiens-cli dialpolicy list`  -  Read Policies
- `netsapiens-cli dialpolicy update`  -  Update Dialpolicy

**domains**  -  Manage domains

- `netsapiens-cli domains count`  -  This API is the same for both Super User and Reseller.
- `netsapiens-cli domains create`  -  This API will allow a new domain to be created.
- `netsapiens-cli domains delete`  -  Delete a Domain
- `netsapiens-cli domains exists`  -  Check if Domain Exists
- `netsapiens-cli domains get`  -  Get Specific Domain
- `netsapiens-cli domains list`  -  This API is the same for both Super User and Reseller.
- `netsapiens-cli domains my`  -  Get My Domain Info
- `netsapiens-cli domains my-answerrules`  -  Read Answerrules for my User
- `netsapiens-cli domains my-contacts`  -  Get My Contacts
- `netsapiens-cli domains my-user`  -  Get My User
- `netsapiens-cli domains update`  -  Update a Domain

**firebase**  -  Manage firebase

- `netsapiens-cli firebase create`  -  Add firebase service account
- `netsapiens-cli firebase list`  -  Read firebase service accounts

**holidays**  -  Manage holidays

- `netsapiens-cli holidays get-by-year`  -  In order to create holidays in a Holiday timeframe or a Custom timeframe
- `netsapiens-cli holidays get-by-year-country`  -  Read Holiday Information by Country and Region
- `netsapiens-cli holidays list`  -  Retrieve a list of supported country codes to be used when reading holiday information.
- `netsapiens-cli holidays list-regions`  -  Retrieve a list of supported region codes by country, to be used when reading holiday information.

**images**  -  Manage images

- `netsapiens-cli images create-with-filename`  -  Create Image from Upload (JSON + Base64 File)
- `netsapiens-cli images delete`  -  Delete an image file
- `netsapiens-cli images get`  -  Read an image file
- `netsapiens-cli images update`  -  Update Image from Upload (Multipart/Mixed Post)

**insight**  -  Manage insight

- `netsapiens-cli insight`  -  Query Data from iNSight

**jwt**  -  Manage jwt

- `netsapiens-cli jwt create`  -  This API will accept username and password and generate a JWT token.
- `netsapiens-cli jwt delete`  -  This API requires a valid JWT and will revoke it by JTI of the current Token.
- `netsapiens-cli jwt delete-jti`  -  > Note: # and anything after is NOT needed, its just to allow multiple examples for the same path/method.
- `netsapiens-cli jwt delete-uid`  -  Revoke JWT(s) by UID (user@domain)
- `netsapiens-cli jwt list`  -  Read Current JWT

**meetings**  -  Manage meetings

- `netsapiens-cli meetings <meeting_registration_id>`  -  Register Meeting

**netsapiens-version**  -  Manage netsapiens version

- `netsapiens-cli netsapiens-version`  -  Read API Version

**phonenumbers**  -  Manage phonenumbers

- `netsapiens-cli phonenumbers count`  -  This path will give you all Phonenumbers (DIDs)
- `netsapiens-cli phonenumbers list`  -  This path will give you all Phonenumbers (DIDs)

**phones**  -  Manage phones

- `netsapiens-cli phones count`  -  Count Mac Addresses
- `netsapiens-cli phones create`  -  Add MAC address
- `netsapiens-cli phones delete`  -  Remove MAC address
- `netsapiens-cli phones get`  -  Read Specific Mac Address
- `netsapiens-cli phones get-servers`  -  Read Provisionable Server Details
- `netsapiens-cli phones list`  -  Read Mac Addresses
- `netsapiens-cli phones list-models`  -  Get list of Supported/Provisionable Models
- `netsapiens-cli phones list-servers`  -  Get list of Provisionable Server Profiles
- `netsapiens-cli phones update`  -  Update MAC address

**resellers**  -  Manage resellers

- `netsapiens-cli resellers count`  -  Count Resellers
- `netsapiens-cli resellers create`  -  Create Reseller
- `netsapiens-cli resellers delete`  -  Delete Reseller
- `netsapiens-cli resellers get`  -  Get Specific Reseller
- `netsapiens-cli resellers list`  -  Get Resellers
- `netsapiens-cli resellers update`  -  Update Reseller

**restore**  -  Manage restore

- `netsapiens-cli restore list`  -  Read Available Restore Points
- `netsapiens-cli restore update`  -  Restore a Specifc Domain Backup

**routecon**  -  Manage routecon

- `netsapiens-cli routecon`  -  Count All Route Connections

**routes**  -  Manage routes

- `netsapiens-cli routes count`  -  Count All Routes
- `netsapiens-cli routes create`  -  Create a Route
- `netsapiens-cli routes delete`  -  Delete A Specific Route
- `netsapiens-cli routes list`  -  Read Routes
- `netsapiens-cli routes update`  -  Update A Specific Route

**sipflow**  -  Manage sipflow

- `netsapiens-cli sipflow`  -  Get Call Trace (SIPFlow) For Call

**smsblocks**  -  Manage smsblocks

- `netsapiens-cli smsblocks`  -  Read SMS Blocks for all Domains

**smsnumbers**  -  Manage smsnumbers

- `netsapiens-cli smsnumbers`  -  Get All SMS Numbers for System

**sso-enroll**  -  Manage sso enroll

- `netsapiens-cli sso-enroll`  -  SSO Enroll

**subscriptions**  -  Manage subscriptions

- `netsapiens-cli subscriptions create`  -  Create a Event Subscription
- `netsapiens-cli subscriptions delete`  -  Delete a subscription
- `netsapiens-cli subscriptions get`  -  Read Event Subscription By Id
- `netsapiens-cli subscriptions list`  -  Read Event Subscriptions
- `netsapiens-cli subscriptions update`  -  Update an Event Subscription

**templates**  -  Manage templates

- `netsapiens-cli templates create-with-filename`  -  Create Template from Upload (JSON + Base64 File)
- `netsapiens-cli templates delete`  -  Delete a template file
- `netsapiens-cli templates get`  -  Read a template file
- `netsapiens-cli templates update`  -  Update Template from Upload (JSON + Base64 File)

**tokens**  -  Manage tokens

- `netsapiens-cli tokens`  -  This API will accept username and password along with a passcode generated via a authenticator application and generate

**video**  -  Manage video

- `netsapiens-cli video`  -  Read Iotum Video Domain Resellers


**fleet**


- `netsapiens-cli fleet seats`  -  From the synced mirror (run sync first): each client domain's users, call queues, auto attendants and conferences against their limits, plus device, number, call and SMS counts, with over-limit and near-limit domains flagged and a flat true-up CSV.

- `netsapiens-cli fleet offline`  -  From the synced mirror (run sync first): phones and softphones that are unregistered, expired, high-latency, never provisioned or not provisioned recently, sorted by client.

- `netsapiens-cli fleet dids`  -  From the synced mirror (run sync first): where every phone number routes, flagging numbers whose target is missing for that routing type, disabled or parked numbers, unverifiable targets, and possible dead ends (a user with no registered phone and voicemail off; answer rules are not inspected).

- `netsapiens-cli fleet e911`  -  From the synced mirror (run sync first): domains, users and devices with no emergency caller ID after inheritance, or with an emergency address ID that does not exist. A data-gap check to feed a Kari's Law / RAY BAUM's Act review, not a full compliance test.

- `netsapiens-cli fleet missed`  -  Live (needs the API key): unanswered inbound calls per user and client over a window, read from CDRs; adds names and registered-phone counts when users and devices are synced.

- `netsapiens-cli fleet queues`  -  Live (needs the API key): call queues across clients ranked by abandonment rate, with service level, available agents and agents whose phones are not registered. Checks up to --max-domains (25) domains and reports when it was capped.

- `netsapiens-cli fleet changes`  -  From the synced mirror: users, devices, numbers and domains added, removed or re-pointed since the baseline. Read-only; record or move the baseline with --rebaseline.


**trace**


- `netsapiens-cli trace`  -  From the synced mirror (run sync first): resolve one phone number, extension, MAC address or email to its client, routing, user, and the user's phones with their registration state.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
netsapiens-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Seat true-up for every client

```bash
netsapiens-cli fleet seats --csv
```

Writes one flat row per domain with a count, limit and status column per metric plus device, number, call and SMS usage for invoice reconciliation.

### Who owns this number?

```bash
netsapiens-cli trace 6025550142 --agent
```

Resolves a DID to its client, routing target, user, and the user's phones with their registration state.

### Lean user list for an agent

```bash
netsapiens-cli domains users list --domain acme.example --all --agent --select user,name-first-name,name-last-name,email,user-scope
```

Users return dozens of fields each; --select keeps only the ones the agent needs, and --all pages past the 100-row default.

### Numbers that route nowhere

```bash
netsapiens-cli fleet dids --problems-only --agent
```

Lists DIDs whose destination user, queue or auto attendant does not exist or cannot be reached.

### E911 data-gap pass

```bash
netsapiens-cli fleet e911 --agent
```

Flags domains, users and devices with no emergency caller ID after inheritance, or with an emergency address ID that does not exist.

## Auth Setup

Credentials the API hands back, such as the key from `apikeys create` or the tokens from `jwt create`, print as `<redacted>`: read a new key's value in the NetSapiens portal.

NetSapiens runs on each provider's own host, so set two variables before anything else: export NETSAPIENS_SERVER=<your platform host, e.g. pbx.yourprovider.example> and export NETSAPIENS_API_KEY=<API key or OAuth2 access token from your provider>. Without NETSAPIENS_SERVER the CLI talks to ns-api.com, which is only the public sandbox. NETSAPIENS_BASE_URL overrides the whole base URL (https://<host>/ns-api/v2) if your provider uses a different path. A reseller-scoped key sees every client domain; an office-manager key sees one domain. Keys can be read-only.

Run `netsapiens-cli doctor` to verify setup.

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
  netsapiens-cli accesslog --agent --select action-type,auditlog-details,by-client
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

- Use `--home <dir>` for one invocation, or set `NETSAPIENS_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `NETSAPIENS_CONFIG_DIR`, `NETSAPIENS_DATA_DIR`, `NETSAPIENS_STATE_DIR`, `NETSAPIENS_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `NETSAPIENS_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `netsapiens-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "netsapiens": {
        "command": "netsapiens-mcp",
        "env": {
          "NETSAPIENS_HOME": "/srv/netsapiens"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `NETSAPIENS_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `NETSAPIENS_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are cached for five minutes under the cache dir. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `NETSAPIENS_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `NETSAPIENS_DATA_SOURCE` sets the `--data-source` default.

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
netsapiens-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "netsapiens-cli learnings confirm 12"] }
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
       materially more, record the divergence via `netsapiens-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `netsapiens-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `netsapiens-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
netsapiens-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
netsapiens-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
netsapiens-cli teach-playbook \
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
netsapiens-cli playbook amend \
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

`netsapiens-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `NETSAPIENS_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
netsapiens-cli feedback "the --since flag is inclusive but docs say exclusive"
netsapiens-cli feedback --stdin < notes.txt
netsapiens-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `NETSAPIENS_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `NETSAPIENS_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
netsapiens-cli profile save briefing --json
netsapiens-cli --profile briefing accesslog
netsapiens-cli profile list --json
netsapiens-cli profile show briefing
netsapiens-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `netsapiens-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add netsapiens-mcp --env NETSAPIENS_SERVER=<your platform host> --env NETSAPIENS_API_KEY=<your key> -- netsapiens-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `netsapiens-mcp --transport http` requires `NETSAPIENS_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `NETSAPIENS_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `NETSAPIENS_MCP_READ_ONLY=1` drops every tool not marked read-only, including the local-write learning tools that only touch the CLI's own store, and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which netsapiens-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   netsapiens-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `netsapiens-cli <command> --help`.
