---
name: datto-rmm
description: "Every Datto RMM API operation, plus a local SQLite fleet store and fleet-wide analytics no other Datto tool has. Trigger phrases: `datto rmm stale devices`, `which endpoints have antivirus disabled`, `datto rmm patch gaps`, `qbr scorecard for a site`, `warranty expiring devices`, `use datto-rmm`, `run datto-rmm-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Datto RMM"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - datto-rmm-cli
    install:
      - kind: script
        bins: [datto-rmm-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-rmm/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-rmm/install.ps1
---

# Datto RMM - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `datto-rmm-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-rmm/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-rmm/install.ps1 | iex
   ```
3. Verify: `datto-rmm-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `datto-rmm-cli` and `datto-rmm-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

datto-rmm-cli mirrors the full Datto RMM v2 API (sites, devices, alerts, jobs, audit, variables) and syncs your whole multi-site fleet into local SQLite. That unlocks questions no single API call can answer: stale devices, alert storms, software sprawl, warranty cliffs, patch and AV gaps, and one-shot QBR scorecards  -  all offline, agent-native, and composable with jq.

## When to Use This CLI

Use this CLI when an agent or MSP technician needs to answer fleet-wide questions about a Datto RMM account  -  which endpoints are stale, unprotected, behind on patches, or out of warranty  -  or to script bulk reads/writes (alerts, variables, quick jobs) that the web UI makes tedious. It is the right tool when the question spans more than one customer site, because the answer lives in the local store, not a single API call.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI for one-off interactive tasks better done in the Datto RMM web console (remote-control sessions, monitor policy editing, component authoring)  -  the API does not expose those surfaces.
- Do not use it for other RMM platforms (NinjaOne, N-central, ConnectWise Automate)  -  each has its own connector.
- Do not use it for Datto BCDR/backup appliances  -  that is the datto-bcdr connector, a separate API.
- Do not use fleet analytics output as live truth without running sync first  -  the local store starts empty and reflects the last sync, not the current console.
- Do not script destructive bulk writes (resetApiKeys, site moves, bulk resolve) without --dry-run/plan-only review first; fleet resolve-storm refuses to act without --confirm.

## Unique Capabilities

### Fleet hygiene that compounds
- **`fleet stale`**  -  Lists every device across all customer sites that hasn't checked in within a chosen window, so you catch dead agents before the customer does.

  _Reach for this when you need the authoritative list of unreachable or abandoned endpoints across the entire fleet, not just one site._

  ```bash
  datto-rmm-cli fleet stale --days 30 --agent --select hostname,siteName,lastSeen,online
  ```
- **`fleet sprawl`**  -  Rolls up audited software across the whole fleet to show how many installs of an app exist and the spread of versions, exposing license waste and unpatched copies.

  _Reach for this when you need a license-true or vulnerability-true count of an application across every endpoint, not a per-device lookup._

  ```bash
  datto-rmm-cli fleet sprawl --name "Google Chrome" --agent
  ```
- **`fleet agent-drift`**  -  Shows which devices run out-of-date RMM agents across the fleet, ranked by how far behind the newest version they are.

  _Reach for this to find endpoints whose stale RMM agent may be missing monitors before troubleshooting visibility gaps._

  ```bash
  datto-rmm-cli fleet agent-drift --agent --select hostname,siteName,cagVersion,lastSeen
  ```
- **`fleet udf`**  -  Rolls one device user-defined field up across every synced site: each distinct value with its device and site counts, how many devices leave it empty, and the devices behind one value or behind the gaps.

  _Reach for this when a UDF carries an MSP convention (contract tier, backup state, owner) and you need every device across every customer that breaks it._

  ```bash
  datto-rmm-cli fleet udf udf7 --agent
  ```

### Alert intelligence
- **`fleet storms`**  -  Ranks the noisiest devices and sites by alert volume over a time window so the NOC can tune monitors instead of drowning in tickets.

  _Reach for this to find the alert-fatigue offenders driving ticket volume before recommending monitor or threshold changes._

  ```bash
  datto-rmm-cli fleet storms --days 7 --top 20 --agent
  ```
- **`fleet av-gaps`**  -  Finds every device fleet-wide where antivirus is missing, disabled, or not running, so security gaps surface before an incident does.

  _Reach for this when you need the definitive list of unprotected endpoints across all customers for a security review or incident sweep._

  ```bash
  datto-rmm-cli fleet av-gaps --status not-running --agent --select hostname,siteName,antivirus.antivirusProduct,antivirus.antivirusStatus
  ```
- **`fleet resolve-storm`**  -  Bulk-resolves the open alerts on the noisiest devices and sites from the storm ranking, so Monday triage ends with a clean queue instead of a worklist.

  _Reach for this when a monitor misfire flooded the queue and you need to act on the ranking, not just look at it; defaults to a plan-only preview, and nothing resolves without --confirm._

  ```bash
  datto-rmm-cli fleet resolve-storm --days 7 --top 10 --agent
  ```
- **`fleet device-timeline`**  -  Stitches one device's alerts and activity-log entries (which surface job, audit, and user actions) into a single chronological timeline from the local store.

  _Reach for this during triage when a device keeps misbehaving and you need its full recent history in one stream, not four console tabs._

  ```bash
  datto-rmm-cli fleet device-timeline ACME-DC01 --days 30 --agent
  ```

### Lifecycle and compliance
- **`fleet patch-gaps`**  -  Ranks every device by missing-patch count across all sites so you remediate the most-exposed endpoints first.

  _Reach for this to prioritize patch remediation fleet-wide by exposure rather than checking one device at a time._

  ```bash
  datto-rmm-cli fleet patch-gaps --min-missing 5 --agent
  ```
- **`fleet warranty`**  -  Surfaces every device whose hardware warranty expires within a window, sorted by date and site, ready to drop into a QBR refresh plan.

  _Reach for this when preparing a hardware-refresh or QBR budget conversation that needs the expiring-warranty list across all customers._

  ```bash
  datto-rmm-cli fleet warranty --within 60 --agent --select hostname,siteName,warrantyDate,deviceType.type
  ```
- **`fleet scorecard`**  -  Produces a one-shot per-site health card fusing device counts, alert volume, patch coverage, AV coverage, warranty exposure, and agent drift for QBRs.

  _Reach for this when you or a vCIO need an instant customer-health summary to open a QBR or flag an at-risk account._

  ```bash
  datto-rmm-cli fleet scorecard "Acme Corporation" --agent
  ```
- **`fleet orphans`**  -  Cross-references long-stale, suspended, or deleted devices that still count against a site so you stop billing and monitoring phantom endpoints.

  _Reach for this when cleaning up a site's device roster or auditing billing accuracy against what is actually alive._

  ```bash
  datto-rmm-cli fleet orphans --stale-days 90 --agent
  ```

### Receipts and data trust
- **`fleet job-results`**  -  Fetches every device's result for one quick job and rolls them into a single pass/fail table, so a 200-device push is verified in one command.

  _Reach for this after any quick-job push to answer which devices errored or printed nothing; fetches live per device, so it needs credentials and a synced cohort (--devices, --site, or --all)._

  ```bash
  datto-rmm-cli fleet job-results 8d3f2c1a-4b6e-4f0a-9c2d-1a2b3c4d5e6f --devices 0a1b2c3d-uid1,4e5f6a7b-uid2 --failed-only --agent
  ```
- **`fleet snapshot`**  -  Freezes the current synced fleet state into a labeled, dated archive so any number you report can be reproduced later as a receipt.

  _Reach for this before a QBR or audit so a disputed number weeks later can be proven against the dated snapshot; run sync first so the receipt reflects fresh data._

  ```bash
  datto-rmm-cli fleet snapshot --label q2-acme --note "QBR baseline"
  ```
- **`fleet diff`**  -  Diffs two fleet snapshots (or a snapshot vs the live store) to show devices added or removed and warranty, patch, AV, and agent-version movement since the baseline.

  _Reach for this in QBR prep to lead with the delta story: what got better, what slipped, what arrived and left; both sides read the synced store, so sync before snapshotting._

  ```bash
  datto-rmm-cli fleet diff --from q1-acme --to q2-acme --agent
  ```
- **`fleet sync-gaps`**  -  Verifies every synced resource pulled all pages by comparing stored row counts against the API's reported totals, so you can trust fleet numbers before reporting them.

  _Reach for this after sync and before any fleet report; an incomplete devices table makes every downstream number quietly wrong._

  ```bash
  datto-rmm-cli fleet sync-gaps --agent
  ```

## Command Reference

**account**  -  Manage account

- `datto-rmm-cli account create-variable`  -  Creates an account variable
- `datto-rmm-cli account delete-variable`  -  Deletes the account variable identified by the given variable Id.
- `datto-rmm-cli account get-components`  -  Fetches the components records of the authenticated user's account.
- `datto-rmm-cli account get-dnet-site-mappings`  -  Fetches the sites records with its mapped dnet network id for the authenticated user's account.
- `datto-rmm-cli account get-sites`  -  Fetches the site records of the authenticated user's account.
- `datto-rmm-cli account get-user`  -  Fetches the authenticated user's account data.
- `datto-rmm-cli account get-user-closed-alerts`  -  If the muted parameter is not provided, both muted and umuted alerts will be queried.
- `datto-rmm-cli account get-user-devices`  -  Fetches the devices of the authenticated user's account.
- `datto-rmm-cli account get-user-open-alerts`  -  If the muted parameter is not provided, both muted and umuted alerts will be queried.
- `datto-rmm-cli account get-users`  -  Fetches the authentication users records of the authenticated user's account.
- `datto-rmm-cli account get-variables`  -  Fetches the account variables.
- `datto-rmm-cli account update-variable`  -  Updates the account variable identified by the given variable Id.

**activity-logs**  -  Manage activity logs

- `datto-rmm-cli activity-logs`  -  Fetches the activity logs.

**alert**  -  Manage alert

- `datto-rmm-cli alert <alertUid>`  -  Fetches data of the alert identified by the given alert Uid.

**audit**  -  Manage audit

- `datto-rmm-cli audit get-device`  -  Fetches audit data of the generic device identified the given device Uid.
- `datto-rmm-cli audit get-device-by-mac-address`  -  Fetches audit data of the generic device(s) identified the given MAC address in format: XXXXXXXXXXXX
- `datto-rmm-cli audit get-device-software`  -  Fetches audited software of the generic device identified the given device Uid.
- `datto-rmm-cli audit get-esxi-host`  -  Fetches audit data of the ESXi host identified the given device Uid.
- `datto-rmm-cli audit get-printer`  -  Fetches audit data of the printer identified the given device Uid.

**device**  -  Manage device

- `datto-rmm-cli device get-by-id`  -  Fetches data of the device identified by the given device Id.
- `datto-rmm-cli device get-by-mac-address`  -  Fetches data of the device(s) identified by the given MAC address in format: XXXXXXXXXXXX
- `datto-rmm-cli device get-by-uid`  -  Fetches data of the device identified by the given device Uid.

**filter**  -  Manage filter

- `datto-rmm-cli filter get-custom`  -  Fetches the custom device filters for the user (using administrator role).
- `datto-rmm-cli filter get-defaults`  -  Fetches the default device filters.

**job**  -  Manage job

- `datto-rmm-cli job <jobUid>`  -  Fetches data of the job identified by the given job Uid.

**site**  -  Manage site

- `datto-rmm-cli site create`  -  Creates a new site in the authenticated user's account.
- `datto-rmm-cli site get`  -  Fetches data of the site (including total number of devices) identified by the given site Uid.
- `datto-rmm-cli site update`  -  Updates the site identified by the given site Uid.

**system**  -  Manage system

- `datto-rmm-cli system get`  -  Fetches the request rate status for the authenticated user's account.
- `datto-rmm-cli system get-pagination-configurations`  -  Fetches the pagination configurations.
- `datto-rmm-cli system get-status`  -  Fetches the system status (start date, status and version).

**user**  -  Manage user

- `datto-rmm-cli user`  -  Resets the authenticated user's API access and secret keys.


**fleet**


- `datto-rmm-cli fleet stale`  -  Lists every device across all customer sites that hasn't checked in within a chosen window, so you catch dead agents before the customer does.

- `datto-rmm-cli fleet storms`  -  Ranks the noisiest devices and sites by alert volume over a time window so the NOC can tune monitors instead of drowning in tickets.

- `datto-rmm-cli fleet patch-gaps`  -  Ranks every device by missing-patch count across all sites so you remediate the most-exposed endpoints first.

- `datto-rmm-cli fleet av-gaps`  -  Finds every device fleet-wide where antivirus is missing, disabled, or not running, so security gaps surface before an incident does.

- `datto-rmm-cli fleet sprawl`  -  Rolls up audited software across the whole fleet to show how many installs of an app exist and the spread of versions, exposing license waste and unpatched copies.

- `datto-rmm-cli fleet warranty`  -  Surfaces every device whose hardware warranty expires within a window, sorted by date and site, ready to drop into a QBR refresh plan.

- `datto-rmm-cli fleet scorecard`  -  Produces a one-shot per-site health card fusing device counts, alert volume, patch coverage, AV coverage, warranty exposure, and agent drift for QBRs.

- `datto-rmm-cli fleet agent-drift`  -  Shows which devices run out-of-date RMM agents across the fleet, ranked by how far behind the newest version they are.

- `datto-rmm-cli fleet orphans`  -  Cross-references long-stale, suspended, or deleted devices that still count against a site so you stop billing and monitoring phantom endpoints.

- `datto-rmm-cli fleet resolve-storm`  -  Bulk-resolves the open alerts on the noisiest devices and sites from the storm ranking, so Monday triage ends with a clean queue instead of a worklist.

- `datto-rmm-cli fleet job-results`  -  Fetches every device's result for one quick job and rolls them into a single pass/fail table, so a 200-device push is verified in one command.

- `datto-rmm-cli fleet snapshot`  -  Freezes the current synced fleet state into a labeled, dated archive so any number you report can be reproduced later as a receipt.

- `datto-rmm-cli fleet diff`  -  Diffs two fleet snapshots (or a snapshot vs the live store) to show devices added or removed and warranty, patch, AV, and agent-version movement since the baseline.

- `datto-rmm-cli fleet device-timeline`  -  Stitches one device's alerts and activity-log entries (which surface job, audit, and user actions) into a single chronological timeline from the local store.

- `datto-rmm-cli fleet sync-gaps`  -  Verifies every synced resource pulled all pages by comparing stored row counts against the API's reported totals, so you can trust fleet numbers before reporting them.

- `datto-rmm-cli fleet udf`  -  Rolls one device user-defined field up across every synced site: each distinct value with its device and site counts, how many devices leave it empty, and the devices behind one value or behind the gaps.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
datto-rmm-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Find unprotected endpoints for a security review

```bash
datto-rmm-cli fleet av-gaps --status not-running --agent --select hostname,siteName,antivirus.antivirusProduct,antivirus.antivirusStatus
```

Returns only the fields an agent needs from the deeply-nested device record (antivirus is a sub-object), so context stays small.

### Patch-remediation worklist by exposure

```bash
datto-rmm-cli fleet patch-gaps --min-missing 5 --agent
```

Every device missing 5+ patches across all sites, ranked, ready to feed a remediation job.

### QBR prep for one customer

```bash
datto-rmm-cli fleet scorecard "Acme Corporation" --agent
```

One health card fusing device counts, open alerts, patch %, AV %, warranty exposure, and agent drift.

### Hardware-refresh budget list

```bash
datto-rmm-cli fleet warranty --within 90 --agent --select hostname,siteName,warrantyDate
```

Devices whose warranty expires within 90 days, sorted by date  -  drop straight into a QBR.

## Auth Setup

Datto RMM uses OAuth 2.0. You have an API key and an API secret key (Setup > Users > generate API keys in the Datto RMM web UI). The first API call mints a bearer token for you from DATTO_RMM_API_KEY + DATTO_RMM_API_SECRET_KEY (no manual token handling) and caches it in the 0600 credential store, bound to that key and host. Pick your region with DATTO_RMM_PLATFORM (pinotage, merlot, concord, vidal, zinfandel, syrah) or set DATTO_RMM_API_URL directly. Tokens last about 100 hours; a fresh one is minted 5 minutes before the cached one expires (run auth logout to drop a revoked token).

Run `datto-rmm-cli doctor` to verify setup.

## Credential safety

- Every API request, the token request and the bearer token go only to the scheme, host and port of the API base URL: `DATTO_RMM_BASE_URL`, or the host `DATTO_RMM_PLATFORM` or `DATTO_RMM_API_URL` names (default `https://merlot-api.centrastage.net/api`). The token request is `POST /auth/oauth/token` on that same host. A request that would carry a credential anywhere else, over plain http, or across a redirect to another host is refused before it is sent, and a next-page link moves only the page number.
- Any output, file, store write or MCP result that would repeat `DATTO_RMM_API_KEY`, `DATTO_RMM_API_SECRET_KEY`, `DATTO_RMM_TOKEN`, a minted token or a credential a response carries (a site proxy password, the keys `resetApiKeys` issues) is replaced by a notice such as "[credkit: withheld from stdout: it carried a credential this program holds]", so rotate API keys in the Datto RMM web UI, where you can read the new pair. Responses are never cached on disk, and those response fields are replaced by `[redacted]` before a record reaches the local store, so a site with a proxy password still syncs.
- Only `credentials.toml` (mode 0600) holds a credential. The API key and secret from the environment are never written to disk. A minted token is cached there (not at all under `DATTO_RMM_NO_CONFIG_WRITE=1`), bound to the host and API key it was minted for: another key mints its own, and a token is never sent to another platform.
- `--dry-run` sends nothing and never mints a token. Over MCP, a call that changes Datto RMM returns a preview and sends only with `confirm: true`, and a tool argument can never name a file to read or write.

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
  datto-rmm-cli alert mock-value --agent
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

- Use `--home <dir>` for one invocation, or set `DATTO_RMM_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `DATTO_RMM_CONFIG_DIR`, `DATTO_RMM_DATA_DIR`, `DATTO_RMM_STATE_DIR`, `DATTO_RMM_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `DATTO_RMM_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `datto-rmm-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "datto-rmm": {
        "command": "datto-rmm-mcp",
        "env": {
          "DATTO_RMM_HOME": "/srv/datto-rmm"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `DATTO_RMM_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `DATTO_RMM_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are never cached on disk. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `DATTO_RMM_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `DATTO_RMM_DATA_SOURCE` sets the `--data-source` default.

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
datto-rmm-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "datto-rmm-cli learnings confirm 12"] }
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
       materially more, record the divergence via `datto-rmm-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `datto-rmm-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `datto-rmm-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
datto-rmm-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
datto-rmm-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
datto-rmm-cli teach-playbook \
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
datto-rmm-cli playbook amend \
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

`datto-rmm-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `DATTO_RMM_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
datto-rmm-cli feedback "the --since flag is inclusive but docs say exclusive"
datto-rmm-cli feedback --stdin < notes.txt
datto-rmm-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `DATTO_RMM_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `DATTO_RMM_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
datto-rmm-cli profile save briefing --json
datto-rmm-cli --profile briefing alert mock-value
datto-rmm-cli profile list --json
datto-rmm-cli profile show briefing
datto-rmm-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `datto-rmm-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add datto-rmm-mcp -- datto-rmm-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `datto-rmm-mcp --transport http` requires `DATTO_RMM_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `DATTO_RMM_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `DATTO_RMM_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which datto-rmm-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   datto-rmm-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `datto-rmm-cli <command> --help`.
